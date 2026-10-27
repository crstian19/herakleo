//! Opens streams in an external media player. The webview cannot decode
//! MPEG-TS, and desktop players handle every codec IPTV providers throw at
//! them. Mirrors wails/internal/player.

use std::{
    env,
    path::PathBuf,
    process::{Child, Command, Stdio},
    sync::Mutex,
};

use serde::Serialize;

/// Used until the user picks another player.
pub const DEFAULT_ID: &str = "mpv";

/// Playback tweaks the UI may set: a closed set of typed values, never
/// free-form arguments, for the same reason as the player allowlist. Zero
/// values mean "leave the player's default". Mirrors Go's player.Options.
#[derive(Debug, Default, Clone, Copy)]
pub struct Options {
    pub software_decoding: bool,
    /// Network cache in seconds; one of CACHE_CHOICES.
    pub cache_secs: u32,
    /// Start volume 1-100.
    pub volume: u32,
}

/// Cache sizes offered in settings (0 = player default).
pub const CACHE_CHOICES: [u32; 5] = [0, 10, 30, 60, 120];

const OPT_SOFTWARE_DECODING: &str = "softwareDecoding";
const OPT_CACHE_SECS: &str = "cacheSecs";
const OPT_VOLUME: &str = "volume";

impl Options {
    /// Rejects values outside the offered choices.
    pub fn validate(&self) -> Result<(), String> {
        if !CACHE_CHOICES.contains(&self.cache_secs) {
            return Err(format!("invalid_option: cacheSecs {}", self.cache_secs));
        }
        if self.volume > 100 {
            return Err(format!("invalid_option: volume {}", self.volume));
        }
        Ok(())
    }
}

struct Known {
    id: &'static str,
    name: &'static str,
    bin: &'static str,
    options: &'static [&'static str],
    args: fn(&str, &str, Options) -> Vec<String>,
}

/// A fixed allowlist on purpose: letting the UI send an arbitrary command
/// would turn any webview bug into code execution. Celluloid, Haruna and
/// SMPlayer get only the URL: they have no stable CLI flags for these
/// options, and passing mpv options through them is exactly the kind of
/// string building the allowlist exists to avoid.
const KNOWN: [Known; 5] = [
    Known {
        id: "mpv",
        name: "mpv",
        bin: "mpv",
        options: &[OPT_SOFTWARE_DECODING, OPT_CACHE_SECS, OPT_VOLUME],
        args: |title, url, o| {
            let mut args = vec![
                "--force-window=immediate".to_string(),
                "--really-quiet".into(),
                // "$$" stops mpv expanding ${path} in a hostile channel name
                // into the window title, which would show the password-bearing URL.
                format!("--title=Herakleo · {}", title.replace('$', "$$")),
            ];
            args.push(
                if o.software_decoding {
                    "--hwdec=no"
                } else {
                    "--hwdec=auto-safe"
                }
                .into(),
            );
            if o.cache_secs > 0 {
                args.push("--cache=yes".into());
                args.push(format!("--cache-secs={}", o.cache_secs));
            }
            if o.volume > 0 {
                args.push(format!("--volume={}", o.volume));
            }
            args.push("--".into());
            args.push(url.into());
            args
        },
    },
    Known {
        id: "vlc",
        name: "VLC",
        bin: "vlc",
        options: &[OPT_SOFTWARE_DECODING, OPT_CACHE_SECS],
        args: |title, url, o| {
            let mut args = vec![format!("--meta-title=Herakleo · {title}")];
            if o.software_decoding {
                args.push("--avcodec-hw=none".into());
            }
            if o.cache_secs > 0 {
                args.push(format!("--network-caching={}", o.cache_secs * 1000));
            }
            args.push(url.into());
            args
        },
    },
    Known {
        id: "celluloid",
        name: "Celluloid",
        bin: "celluloid",
        options: &[],
        args: |_, url, _| vec![url.into()],
    },
    Known {
        id: "haruna",
        name: "Haruna",
        bin: "haruna",
        options: &[],
        args: |_, url, _| vec![url.into()],
    },
    Known {
        id: "smplayer",
        name: "SMPlayer",
        bin: "smplayer",
        options: &[],
        args: |_, url, _| vec![url.into()],
    },
];

/// A supported player for the settings screen.
#[derive(Serialize)]
pub struct Info {
    id: &'static str,
    name: &'static str,
    installed: bool,
    /// Which Options this player honours; the UI greys out the rest.
    options: &'static [&'static str],
}

/// Every supported player and whether it is on PATH.
pub fn available() -> Vec<Info> {
    KNOWN
        .iter()
        .map(|p| Info {
            id: p.id,
            name: p.name,
            installed: find_bin(p.bin).is_some(),
            options: p.options,
        })
        .collect()
}

pub fn known(id: &str) -> bool {
    KNOWN.iter().any(|p| p.id == id)
}

/// Keeps a single instance: starting a channel replaces the previous one.
#[derive(Default)]
pub struct Player {
    child: Mutex<Option<Child>>,
}

impl Player {
    /// Opens the stream in the chosen player, closing whatever was playing before.
    pub fn play(&self, player_id: &str, title: &str, url: &str, o: Options) -> Result<(), String> {
        // Checked here too because caches written before the parser filtered
        // schemes may hold anything. It also guarantees the URL starts with a
        // scheme, so players without "--" can never read it as a flag.
        if !crate::playlist::is_stream_url(url) {
            return Err("stream_url_forbidden".into());
        }
        let p = KNOWN
            .iter()
            .find(|p| p.id == player_id)
            .ok_or_else(|| format!("unknown_player: {player_id}"))?;
        let bin = find_bin(p.bin).ok_or_else(|| format!("player_missing: {}", p.name))?;
        let mut guard = self.child.lock().map_err(|_| "state_locked")?;
        stop(&mut guard);

        // Output is discarded (as Go's exec does) because players print the
        // password-bearing URL, and from a desktop launcher stdout ends up in
        // the journal.
        let child = Command::new(bin)
            .args((p.args)(title, url, o))
            .stdin(Stdio::null())
            .stdout(Stdio::null())
            .stderr(Stdio::null())
            .spawn()
            .map_err(|e| format!("player_launch_failed: {}: {e}", p.name))?;
        *guard = Some(child);
        Ok(())
    }

    pub fn stop(&self) {
        if let Ok(mut guard) = self.child.lock() {
            stop(&mut guard);
        }
    }
}

/// Kills and reaps the previous player so it never lingers as a zombie.
fn stop(slot: &mut Option<Child>) {
    if let Some(mut child) = slot.take() {
        let _ = child.kill();
        let _ = child.wait();
    }
}

/// Resolves a binary on PATH like Go's exec.LookPath, which also refuses
/// relative entries such as "." so a stray ./mpv in the working directory never runs.
fn find_bin(name: &str) -> Option<PathBuf> {
    env::split_paths(&env::var_os("PATH")?)
        .filter(|dir| dir.is_absolute())
        .map(|dir| dir.join(name))
        .find(|p| p.is_file())
}

#[cfg(test)]
mod tests {
    use super::*;

    /// Same expectations as player_test.go.
    #[test]
    fn args_per_player() {
        let mpv = (KNOWN[0].args)("a$b", "http://s/1", Options::default());
        assert_eq!(mpv[2], "--title=Herakleo · a$$b");
        assert_eq!(&mpv[3..], ["--hwdec=auto-safe", "--", "http://s/1"]);
        assert_eq!(
            (KNOWN[1].args)("t", "http://s/1", Options::default()),
            ["--meta-title=Herakleo · t", "http://s/1"]
        );
        assert!(known("vlc") && !known("sh"));
    }

    /// Same expectations as TestOptionsPerPlayer in player_test.go.
    #[test]
    fn options_per_player() {
        let o = Options {
            software_decoding: true,
            cache_secs: 30,
            volume: 70,
        };
        let mpv = (KNOWN[0].args)("t", "http://s/1", o);
        assert_eq!(
            &mpv[3..],
            [
                "--hwdec=no",
                "--cache=yes",
                "--cache-secs=30",
                "--volume=70",
                "--",
                "http://s/1"
            ]
        );
        let vlc = (KNOWN[1].args)("t", "http://s/1", o);
        assert_eq!(
            &vlc[1..],
            ["--avcodec-hw=none", "--network-caching=30000", "http://s/1"]
        );
        assert_eq!((KNOWN[2].args)("t", "http://s/1", o), ["http://s/1"]);
        for bad in [Options { cache_secs: 7, ..o }, Options { volume: 101, ..o }] {
            assert!(bad.validate().is_err(), "{bad:?}");
        }
        assert!(o.validate().is_ok());
    }
}
