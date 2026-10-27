//! Turns M3U files and Xtream Codes accounts into channels.
//! Mirrors wails/internal/playlist; keep both in step.

use std::{collections::HashMap, fs, io::Read, time::Duration};

use serde::Deserialize;
use serde_json::Value;

/// Downloads above this are a misconfigured URL or a hostile server.
pub const MAX_PLAYLIST_BYTES: usize = 256 << 20;
/// Bounds memory for hostile lists; real ones stay well below.
pub const MAX_ENTRIES: usize = 500_000;
/// Absurd lines are skipped rather than failing the import. Generous because
/// some #EXTINF lines carry base64 logos.
const MAX_LINE: usize = 1 << 20;

/// What mpv may open. Anything else (file paths, edl://, av://v4l2:...,
/// lavf://) would let a playlist make mpv read local files or devices.
const STREAM_SCHEMES: [&str; 7] = ["http", "https", "rtmp", "rtmps", "rtsp", "udp", "rtp"];

/// Whether a stream URL is safe to hand to mpv.
pub fn is_stream_url(raw: &str) -> bool {
    reqwest::Url::parse(raw).is_ok_and(|u| {
        STREAM_SCHEMES.contains(&u.scheme()) && u.host_str().is_some_and(|h| !h.is_empty())
    })
}

/// Logos are limited to web images, matching the Go build which has no CSP.
fn logo_or_empty(raw: String) -> String {
    let lower = raw.to_ascii_lowercase();
    if lower.starts_with("http://")
        || lower.starts_with("https://")
        || lower.starts_with("data:image/")
    {
        raw
    } else {
        String::new()
    }
}

/// One playable item from a playlist, before it is assigned an ID.
#[derive(Debug, Default, Clone, PartialEq)]
pub struct Entry {
    pub name: String,
    pub group: String,
    pub logo: String,
    pub url: String,
}

/// Lenient extended-M3U parser: unknown directives are skipped and a URL
/// without a preceding #EXTINF still becomes an entry named after the URL.
pub fn parse_m3u(text: &str) -> Vec<Entry> {
    let mut entries = Vec::new();
    let mut pending = Entry::default();
    let mut has_info = false;

    // A BOM is not whitespace to trim(); Windows editors add one.
    for line in text
        .lines()
        .map(|l| l.trim_start_matches('\u{feff}').trim())
    {
        if entries.len() >= MAX_ENTRIES {
            break;
        }
        if line.is_empty() || line.len() > MAX_LINE {
            continue;
        }
        if let Some(rest) = line.strip_prefix("#EXTINF:") {
            pending = parse_extinf(rest);
            has_info = true;
        } else if let Some(rest) = line.strip_prefix("#EXTGRP:") {
            if pending.group.is_empty() {
                pending.group = rest.trim().to_string();
            }
        } else if line.starts_with('#') {
            continue;
        } else {
            let mut e = std::mem::take(&mut pending);
            e.url = line.to_string();
            if !has_info || e.name.is_empty() {
                e.name = line.to_string();
            }
            e.logo = logo_or_empty(e.logo);
            if is_stream_url(&e.url) {
                entries.push(e);
            }
            has_info = false;
        }
    }
    entries
}

/// Parses `-1 tvg-logo="x" group-title="y",Display Name`. The name starts
/// after the first comma outside quotes, since titles often contain commas.
fn parse_extinf(s: &str) -> Entry {
    let mut e = Entry::default();
    let mut in_quote = false;
    let mut split = None;
    for (i, c) in s.char_indices() {
        match c {
            '"' => in_quote = !in_quote,
            ',' if !in_quote => {
                split = Some(i);
                break;
            }
            _ => {}
        }
    }
    let attrs = match split {
        Some(i) => {
            e.name = s[i + 1..].trim().to_string();
            &s[..i]
        }
        None => s,
    };

    for (key, val) in attributes(attrs) {
        match key.to_ascii_lowercase().as_str() {
            "group-title" => e.group = val,
            "tvg-logo" => e.logo = val,
            "tvg-name" if e.name.is_empty() => e.name = val,
            _ => {}
        }
    }
    e
}

/// Extracts key="value" pairs, ignoring the leading duration.
fn attributes(mut s: &str) -> Vec<(String, String)> {
    let mut out = Vec::new();
    while let Some(eq) = s.find("=\"") {
        let key = &s[..eq];
        let key = key.rsplit([' ', '\t']).next().unwrap_or(key);
        let rest = &s[eq + 2..];
        let Some(end) = rest.find('"') else { break };
        out.push((key.to_string(), rest[..end].trim().to_string()));
        s = &rest[end + 1..];
    }
    out
}

/// Rejects anything that is not an absolute http(s) URL, so a user-supplied
/// source cannot read local files or reach odd schemes.
pub fn validate_http_url(raw: &str) -> Result<reqwest::Url, String> {
    const MSG: &str = "url_not_http";
    let u = reqwest::Url::parse(raw).map_err(|_| MSG.to_string())?;
    if !matches!(u.scheme(), "http" | "https") || u.host_str().is_none_or(str::is_empty) {
        return Err(MSG.into());
    }
    Ok(u)
}

/// Downloads a URL into memory, bounded by MAX_PLAYLIST_BYTES. Errors never
/// echo the URL because Xtream URLs carry the account password.
async fn get(url: &str) -> Result<Vec<u8>, String> {
    let client = reqwest::Client::builder()
        .timeout(Duration::from_secs(120))
        // Some providers block unknown UAs; present as a media player.
        .user_agent("VLC/3.0.21 LibVLC/3.0.21")
        .build()
        .map_err(|e| e.to_string())?;

    let mut resp = client
        .get(url)
        .send()
        .await
        .map_err(|e| format!("download_failed: {}", e.without_url()))?;
    if resp.status() != reqwest::StatusCode::OK {
        return Err(format!("server_status: {}", resp.status()));
    }

    let mut body = Vec::new();
    while let Some(chunk) = resp
        .chunk()
        .await
        .map_err(|e| format!("download_interrupted: {}", e.without_url()))?
    {
        if body.len() + chunk.len() > MAX_PLAYLIST_BYTES {
            return Err("playlist_too_large".into());
        }
        body.extend_from_slice(&chunk);
    }
    Ok(body)
}

/// Downloads and parses a remote playlist.
pub async fn m3u_from_url(url: &str) -> Result<Vec<Entry>, String> {
    validate_http_url(url)?;
    let body = get(url).await?;
    Ok(parse_m3u(&String::from_utf8_lossy(&body)))
}

/// Parses a playlist from disk.
pub fn m3u_from_file(path: &str) -> Result<Vec<Entry>, String> {
    let f = fs::File::open(path).map_err(|e| format!("file_open_failed: {e}"))?;
    let meta = f.metadata().map_err(|e| e.to_string())?;
    if !meta.is_file() {
        return Err("not_regular_file".into());
    }
    if meta.len() > MAX_PLAYLIST_BYTES as u64 {
        return Err("playlist_too_large".into());
    }
    let mut buf = Vec::new();
    f.take(MAX_PLAYLIST_BYTES as u64)
        .read_to_end(&mut buf)
        .map_err(|e| e.to_string())?;
    Ok(parse_m3u(&String::from_utf8_lossy(&buf)))
}

/// An Xtream Codes account.
pub struct Xtream<'a> {
    pub server: &'a str,
    pub username: &'a str,
    pub password: &'a str,
}

#[derive(Deserialize)]
struct XtreamCategory {
    #[serde(default)]
    category_id: Value,
    #[serde(default)]
    category_name: Option<String>,
}

#[derive(Deserialize)]
struct XtreamStream {
    #[serde(default)]
    name: Option<String>,
    #[serde(default)]
    stream_id: Value,
    #[serde(default)]
    stream_icon: Option<String>,
    #[serde(default)]
    category_id: Value,
}

impl Xtream<'_> {
    /// The account as shown in the UI, without credentials.
    pub fn location(&self) -> String {
        self.server.trim().trim_end_matches('/').to_string()
    }

    /// Lists the account's live TV streams.
    pub async fn live_channels(&self) -> Result<Vec<Entry>, String> {
        let base = self.base()?;
        let cats: Vec<XtreamCategory> = self.call(&base, "get_live_categories").await?;
        let groups: HashMap<String, String> = cats
            .into_iter()
            .map(|c| (raw_id(&c.category_id), c.category_name.unwrap_or_default()))
            .collect();

        let streams: Vec<XtreamStream> = self.call(&base, "get_live_streams").await?;
        let user = path_escape(self.username);
        let pass = path_escape(self.password);
        Ok(streams
            .into_iter()
            .filter_map(|s| {
                let id = raw_id(&s.stream_id);
                (!id.is_empty()).then(|| Entry {
                    name: s.name.unwrap_or_default().trim().to_string(),
                    group: groups
                        .get(&raw_id(&s.category_id))
                        .cloned()
                        .unwrap_or_default(),
                    logo: logo_or_empty(s.stream_icon.unwrap_or_default()),
                    url: format!("{base}/live/{user}/{pass}/{}.ts", path_escape(&id)),
                })
            })
            .take(MAX_ENTRIES)
            .collect())
    }

    fn base(&self) -> Result<String, String> {
        // Returned as typed (trimmed), not re-serialised: Url normalises host
        // case and default ports differently from Go, and the stream URL feeds
        // the channel ID, so both builds must start from the same string.
        let loc = self.location();
        let u = validate_http_url(&loc)?;
        if !u.username().is_empty() || u.password().is_some() {
            return Err("credentials_in_url".into());
        }
        Ok(loc)
    }

    async fn call<T: for<'de> Deserialize<'de>>(
        &self,
        base: &str,
        action: &str,
    ) -> Result<T, String> {
        let mut url =
            reqwest::Url::parse(&format!("{base}/player_api.php")).map_err(|e| e.to_string())?;
        url.query_pairs_mut()
            .append_pair("username", self.username)
            .append_pair("password", self.password)
            .append_pair("action", action);
        let body = get(url.as_str()).await?;
        // Panels answer bad credentials with an object (or nothing) instead of a list.
        if !String::from_utf8_lossy(&body).trim_start().starts_with('[') {
            return Err("xtream_rejected".into());
        }
        serde_json::from_slice(&body).map_err(|_| "xtream_invalid_response".into())
    }
}

/// Subscription info an Xtream panel reports.
#[derive(Debug, Default, Clone, PartialEq)]
pub struct Account {
    /// Unix seconds; 0 means unlimited or unknown.
    pub expires_at: i64,
    /// As the panel words it, e.g. "Active", "Expired", "Banned".
    pub status: String,
}

impl Xtream<'_> {
    /// Asks the panel for the account's expiry and status (the player API
    /// without an action). Mirrors Go's AccountInfo.
    pub async fn account_info(&self) -> Result<Account, String> {
        #[derive(Deserialize, Default)]
        struct UserInfo {
            #[serde(default)]
            exp_date: Value,
            #[serde(default)]
            status: Option<String>,
        }
        #[derive(Deserialize)]
        struct Resp {
            #[serde(default)]
            user_info: UserInfo,
        }
        let base = self.base()?;
        let mut url =
            reqwest::Url::parse(&format!("{base}/player_api.php")).map_err(|e| e.to_string())?;
        url.query_pairs_mut()
            .append_pair("username", self.username)
            .append_pair("password", self.password);
        let body = get(url.as_str()).await?;
        let resp: Resp =
            serde_json::from_slice(&body).map_err(|_| "xtream_invalid_response".to_string())?;
        Ok(Account {
            expires_at: raw_id(&resp.user_info.exp_date)
                .parse::<i64>()
                .unwrap_or(0)
                .max(0),
            status: resp.user_info.status.unwrap_or_default().trim().to_string(),
        })
    }
}

/// Normalises IDs that panels send either as numbers or as strings.
fn raw_id(v: &Value) -> String {
    match v {
        Value::String(s) => s.trim().to_string(),
        Value::Number(n) => n.to_string(),
        _ => String::new(),
    }
}

/// Same escaping as Go's url.PathEscape, so both builds produce identical
/// stream URLs (and therefore identical channel IDs).
fn path_escape(s: &str) -> String {
    let mut out = String::with_capacity(s.len());
    for b in s.bytes() {
        if b.is_ascii_alphanumeric() || b"-_.~$&+=:@".contains(&b) {
            out.push(b as char);
        } else {
            out.push_str(&format!("%{b:02X}"));
        }
    }
    out
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn parses_extinf_with_comma_in_attribute_and_title() {
        let list = "#EXTM3U\n#EXTINF:-1 tvg-logo=\"http://l/a.png\" group-title=\"News, World\",BBC, One\nhttp://s/1\n";
        assert_eq!(
            parse_m3u(list),
            vec![Entry {
                name: "BBC, One".into(),
                group: "News, World".into(),
                logo: "http://l/a.png".into(),
                url: "http://s/1".into(),
            }]
        );
    }

    #[test]
    fn drops_non_network_streams_and_logos() {
        let list = "#EXTINF:-1 tvg-logo=\"file:///etc/x\",Cam\nav://v4l2:/dev/video0\n#EXTINF:-1 tvg-logo=\"javascript:x\",Ok\nhttp://s/ok\n/etc/passwd\n";
        let got = parse_m3u(list);
        assert_eq!(got.len(), 1);
        assert_eq!((got[0].name.as_str(), got[0].logo.as_str()), ("Ok", ""));
    }

    #[test]
    fn bom_and_xtream_ids() {
        let got = parse_m3u("\u{feff}#EXTM3U\n#EXTINF:-1,A\nhttp://s/a\n");
        assert_eq!((got.len(), got[0].name.as_str()), (1, "A"));
        for (raw, want) in [
            (r#"" 12 ""#, "12"),
            ("42", "42"),
            ("null", ""),
            ("true", ""),
            ("{}", ""),
        ] {
            assert_eq!(raw_id(&serde_json::from_str(raw).unwrap()), want, "{raw}");
        }
    }

    #[test]
    fn bare_url_and_extgrp() {
        let list = "#EXTINF:-1,A\n#EXTGRP:Sports\n#EXTVLCOPT:x=y\nhttp://s/a\nhttp://s/b\n";
        let got = parse_m3u(list);
        assert_eq!(got[0].group, "Sports");
        assert_eq!(got[1].name, "http://s/b");
    }

    #[test]
    fn path_escape_matches_go() {
        assert_eq!(path_escape("a b/c?d"), "a%20b%2Fc%3Fd");
    }
}
