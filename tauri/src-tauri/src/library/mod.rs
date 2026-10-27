//! Sources, cached channels, favourites, hidden items, history and shared
//! settings, persisted as JSON under the config dir. Mirrors
//! wails/internal/library file by file, so both apps open the same library,
//! possibly at the same time (see sync.rs).

mod colors;
mod curation;
mod query;
mod sync;
#[cfg(test)]
mod tests;

use std::{
    collections::{HashMap, HashSet},
    fs,
    io::{self, Write},
    os::unix::fs::{DirBuilderExt, OpenOptionsExt, PermissionsExt},
    path::{Path, PathBuf},
    sync::{Mutex, RwLock},
    time::{SystemTime, UNIX_EPOCH},
};

use serde::{Deserialize, Serialize};

pub use curation::{read_backup, write_backup};
pub use query::{Group, Page, Query};

use crate::playlist::{self, Account, Entry, Xtream};

#[derive(Debug, Clone, Copy, PartialEq, Eq, Serialize, Deserialize)]
#[serde(rename_all = "snake_case")]
pub enum Kind {
    M3uUrl,
    M3uFile,
    Xtream,
}

/// Persisted form; holds credentials and never leaves the backend.
#[derive(Debug, Clone, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
struct Source {
    id: String,
    name: String,
    kind: Kind,
    #[serde(default, skip_serializing_if = "String::is_empty")]
    url: String,
    #[serde(default, skip_serializing_if = "String::is_empty")]
    path: String,
    #[serde(default, skip_serializing_if = "String::is_empty")]
    server: String,
    #[serde(default, skip_serializing_if = "String::is_empty")]
    username: String,
    #[serde(default, skip_serializing_if = "String::is_empty")]
    password: String,
    #[serde(default)]
    channel_count: usize,
    #[serde(default)]
    updated_at: i64,
    /// Disabled rather than enabled so lists saved before this field existed stay on.
    #[serde(default, skip_serializing_if = "is_false")]
    disabled: bool,
    /// Xtream account info from the last refresh; zero/empty when unknown.
    #[serde(default, skip_serializing_if = "is_zero")]
    expires_at: i64,
    #[serde(default, skip_serializing_if = "String::is_empty")]
    account_status: String,
}

/// Persisted form; the URL stays in the backend since Xtream URLs embed the password.
#[derive(Debug, Clone, Serialize, Deserialize)]
struct StoredChannel {
    id: String,
    name: String,
    group: String,
    logo: String,
    url: String,
    /// `name` in lower case, computed once at load (`with_keys`) so search
    /// and sort never re-lowercase 65k names per query. Not persisted.
    #[serde(skip)]
    lower: String,
}

/// Fills the derived fields of freshly loaded channels.
fn with_keys(mut chs: Vec<StoredChannel>) -> Vec<StoredChannel> {
    for c in &mut chs {
        c.lower = c.name.to_lowercase();
    }
    chs
}

/// One played channel, newest first in the history list.
#[derive(Debug, Clone, Serialize, Deserialize)]
struct HistoryEntry {
    id: String,
    at: i64,
}

#[derive(Default, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
struct State {
    #[serde(default)]
    sources: Vec<Source>,
    #[serde(default)]
    favorites: Vec<String>,
    #[serde(default)]
    settings: Settings,
    #[serde(default)]
    hidden_channels: Vec<String>,
    #[serde(default)]
    hidden_groups: Vec<String>,
    #[serde(default)]
    history: Vec<HistoryEntry>,
}

/// App preferences both builds share, unlike the purely visual ones the
/// frontend keeps in localStorage.
#[derive(Debug, Default, Clone, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct Settings {
    /// A player ID from player.rs; empty means the default.
    #[serde(default)]
    pub player: String,
    #[serde(default)]
    pub refresh_on_start: bool,
    // Playback options; validated in player.rs before they get here.
    #[serde(default)]
    pub software_decoding: bool,
    #[serde(default)]
    pub cache_secs: u32,
    #[serde(default)]
    pub volume: u32,
}

/// Changes only the fields that are set.
#[derive(Debug, Default, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct SettingsPatch {
    pub player: Option<String>,
    pub refresh_on_start: Option<bool>,
    pub software_decoding: Option<bool>,
    pub cache_secs: Option<u32>,
    pub volume: Option<u32>,
}

/// What the UI sends to add a source.
#[derive(Debug, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct NewSource {
    kind: Kind,
    name: String,
    #[serde(default)]
    url: String,
    #[serde(default)]
    path: String,
    #[serde(default)]
    server: String,
    #[serde(default)]
    username: String,
    #[serde(default)]
    password: String,
}

/// A source as the UI sees it.
#[derive(Debug, Serialize)]
#[serde(rename_all = "camelCase")]
pub struct SourceInfo {
    id: String,
    name: String,
    kind: Kind,
    location: String,
    channel_count: usize,
    updated_at: i64,
    enabled: bool,
    expires_at: i64,
    account_status: String,
}

#[derive(Default)]
struct Inner {
    sources: Vec<Source>,
    favorites: HashSet<String>,
    hidden_channels: HashSet<String>,
    hidden_groups: HashSet<String>,
    history: Vec<HistoryEntry>,
    settings: Settings,
    channels: HashMap<String, Vec<StoredChannel>>,
    /// Changes whenever `channels` does; invalidates the sorted cache (query.rs).
    chan_gen: u64,
    /// Modification times of what we last read or wrote, to notice the
    /// other build changing the files underneath us (sync.rs).
    state_mod: Option<SystemTime>,
    chan_mod: HashMap<String, Option<SystemTime>>,
}

/// Shared across Tauri commands; never hold the lock across an `.await`.
pub struct Library {
    dir: PathBuf,
    inner: RwLock<Inner>,
    /// Logo accent cache; separate lock because it is filled by slow
    /// downloads. Loaded lazily from colors.json.
    colors: Mutex<Option<HashMap<String, String>>>,
    /// Every channel in A→Z order as (source index, channel index), tagged
    /// with the `chan_gen` it was built for (query.rs).
    sorted: Mutex<Option<SortCache>>,
}

/// (`chan_gen` it was built for, channel positions in A→Z order).
type SortCache = (u64, Vec<(usize, usize)>);

type Result<T> = std::result::Result<T, String>;

const LOCKED: &str = "state_locked";

/// A downloaded source plus Xtream account info, if any.
struct Loaded {
    entries: Vec<Entry>,
    account: Account,
}

impl Library {
    /// Loads the library from `dir`, creating it on first run.
    pub fn open(dir: PathBuf) -> Result<Self> {
        // 0700 because library.json holds passwords; existing dirs are
        // tightened too since the Go build may have created them first.
        for d in [dir.clone(), dir.join("channels")] {
            fs::DirBuilder::new()
                .recursive(true)
                .mode(0o700)
                .create(&d)
                .and_then(|()| fs::set_permissions(&d, fs::Permissions::from_mode(0o700)))
                .map_err(|e| e.to_string())?;
        }
        let lib = Self {
            dir,
            inner: RwLock::new(Inner::default()),
            colors: Mutex::new(None),
            sorted: Mutex::new(None),
        };
        {
            let mut inner = lib.inner.write().map_err(|_| LOCKED)?;
            lib.reload_locked(&mut inner)?;
            lib.remove_orphans(&inner);
        }
        Ok(lib)
    }

    /// Downloads a new source and stores it. Nothing is saved if loading fails.
    pub async fn add(&self, n: NewSource) -> Result<SourceInfo> {
        let mut s = Source {
            id: new_id()?,
            name: n.name.trim().to_string(),
            kind: n.kind,
            url: n.url.trim().to_string(),
            path: n.path,
            server: n.server.trim().to_string(),
            username: n.username.trim().to_string(),
            password: n.password,
            channel_count: 0,
            updated_at: 0,
            disabled: false,
            expires_at: 0,
            account_status: String::new(),
        };
        if s.name.is_empty() {
            return Err("name_required".into());
        }
        let loaded = load(&s).await?;

        let mut inner = self.write()?;
        self.store_channels(&mut inner, &mut s, loaded)?;
        let info = s.info();
        inner.sources.push(s);
        if let Err(e) = self.save_state(&mut inner) {
            let s = inner.sources.pop().expect("just pushed");
            inner.channels.remove(&s.id);
            inner.chan_gen += 1;
            let _ = fs::remove_file(channels_path(&self.dir, &s.id));
            return Err(e);
        }
        Ok(info)
    }

    /// Re-downloads a source. Favourites survive because channel IDs derive
    /// from the stream URL.
    pub async fn refresh(&self, id: &str) -> Result<SourceInfo> {
        let snapshot = self.snapshot(id)?.ok_or("source_not_found")?;
        let loaded = load(&snapshot).await?;

        let mut inner = self.write()?;
        let idx = position(&inner, id).ok_or("source_removed")?;
        let mut s = inner.sources[idx].clone();
        self.store_channels(&mut inner, &mut s, loaded)?;
        let info = s.info();
        inner.sources[idx] = s;
        self.save_state(&mut inner)?;
        Ok(info)
    }

    /// Edits a source. Empty fields keep their current value, so the UI never
    /// reads credentials back. Changing where or how the list is loaded
    /// re-downloads it first and saves nothing if that fails; a rename alone is
    /// instant. New stream URLs mean new channel IDs, so favourites of an
    /// Xtream source do not survive a credential change.
    pub async fn update(&self, id: &str, n: NewSource) -> Result<SourceInfo> {
        let mut next = self.snapshot(id)?.ok_or("source_not_found")?;

        let mut reload = false;
        let mut set = |dst: &mut String, v: &str| {
            if !v.is_empty() && v != dst.as_str() {
                *dst = v.to_string();
                reload = true;
            }
        };
        set(&mut next.url, n.url.trim());
        set(&mut next.path, &n.path);
        set(&mut next.server, n.server.trim());
        set(&mut next.username, n.username.trim());
        set(&mut next.password, &n.password);
        if !n.name.trim().is_empty() {
            next.name = n.name.trim().to_string();
        }

        let loaded = if reload {
            Some(load(&next).await?)
        } else {
            None
        };

        let mut inner = self.write()?;
        let idx = position(&inner, id).ok_or("source_removed")?;
        // Keep fields another call may have changed meanwhile.
        next.disabled = inner.sources[idx].disabled;
        if let Some(loaded) = loaded {
            self.store_channels(&mut inner, &mut next, loaded)?;
        }
        let info = next.info();
        inner.sources[idx] = next;
        self.save_state(&mut inner)?;
        Ok(info)
    }

    /// Turns a source on or off without deleting it. Disabled sources keep
    /// their cache but are left out of channels, groups and refresh-all.
    pub fn set_enabled(&self, id: &str, enabled: bool) -> Result<SourceInfo> {
        let mut inner = self.write()?;
        let idx = position(&inner, id).ok_or("source_not_found")?;
        inner.sources[idx].disabled = !enabled;
        let info = inner.sources[idx].info();
        self.save_state(&mut inner)?;
        Ok(info)
    }

    /// Deletes a source and everything that points at its channels.
    pub fn remove(&self, id: &str) -> Result<()> {
        let mut inner = self.write()?;
        let idx = position(&inner, id).ok_or("source_not_found")?;
        inner.sources.remove(idx);
        let prefix = format!("{id}-");
        inner.favorites.retain(|f| !f.starts_with(&prefix));
        inner.hidden_channels.retain(|f| !f.starts_with(&prefix));
        inner.history.retain(|h| !h.id.starts_with(&prefix));
        inner.channels.remove(id);
        inner.chan_mod.remove(id);
        inner.chan_gen += 1;
        match fs::remove_file(channels_path(&self.dir, id)) {
            Err(e) if e.kind() != io::ErrorKind::NotFound => return Err(e.to_string()),
            _ => {}
        }
        self.save_state(&mut inner)
    }

    /// Every source in insertion order, enabled or not.
    pub fn sources(&self) -> Result<Vec<SourceInfo>> {
        let inner = self.read()?;
        Ok(inner.sources.iter().map(Source::info).collect())
    }

    pub fn set_favorite(&self, channel_id: &str, fav: bool) -> Result<()> {
        let mut inner = self.write()?;
        set_flag(&mut inner.favorites, channel_id, fav);
        self.save_state(&mut inner)
    }

    pub fn settings(&self) -> Result<Settings> {
        Ok(self.read()?.settings.clone())
    }

    /// Applies a patch; the caller validates the player ID and options.
    pub fn update_settings(&self, p: SettingsPatch) -> Result<Settings> {
        let mut inner = self.write()?;
        if let Some(player) = p.player {
            inner.settings.player = player;
        }
        if let Some(on) = p.refresh_on_start {
            inner.settings.refresh_on_start = on;
        }
        if let Some(on) = p.software_decoding {
            inner.settings.software_decoding = on;
        }
        if let Some(secs) = p.cache_secs {
            inner.settings.cache_secs = secs;
        }
        if let Some(v) = p.volume {
            inner.settings.volume = v;
        }
        self.save_state(&mut inner)?;
        Ok(inner.settings.clone())
    }

    /// Resolves a channel ID to (name, stream URL) for the player.
    pub fn stream_url(&self, channel_id: &str) -> Result<(String, String)> {
        let inner = self.read()?;
        find_channel(&inner, channel_id)
            .map(|c| (c.name.clone(), c.url.clone()))
            .ok_or_else(|| "channel_not_found".into())
    }

    /// Assigns IDs, persists the cache and updates `s`. Caller holds the write lock.
    fn store_channels(&self, inner: &mut Inner, s: &mut Source, loaded: Loaded) -> Result<()> {
        if loaded.entries.is_empty() {
            return Err("playlist_empty".into());
        }
        let mut seen = HashSet::with_capacity(loaded.entries.len());
        let chs: Vec<StoredChannel> = loaded
            .entries
            .into_iter()
            .filter_map(|e| {
                let id = channel_id(&s.id, &e.url);
                seen.insert(id.clone()).then(|| StoredChannel {
                    id,
                    lower: e.name.to_lowercase(),
                    name: e.name,
                    group: e.group,
                    logo: e.logo,
                    url: e.url,
                })
            })
            .collect();

        let data = serde_json::to_vec(&chs).map_err(|e| e.to_string())?;
        let path = channels_path(&self.dir, &s.id);
        write_atomic(&path, &data)?;
        s.channel_count = chs.len();
        s.updated_at = now();
        s.expires_at = loaded.account.expires_at;
        s.account_status = loaded.account.status;
        inner.chan_mod.insert(s.id.clone(), mod_time(&path));
        inner.channels.insert(s.id.clone(), chs);
        inner.chan_gen += 1;
        Ok(())
    }

    /// Persists everything but the channel caches. Caller holds the write lock.
    fn save_state(&self, inner: &mut Inner) -> Result<()> {
        let st = State {
            sources: inner.sources.clone(),
            favorites: sorted(&inner.favorites),
            settings: inner.settings.clone(),
            hidden_channels: sorted(&inner.hidden_channels),
            hidden_groups: sorted(&inner.hidden_groups),
            history: inner.history.clone(),
        };
        let data = serde_json::to_vec_pretty(&st).map_err(|e| e.to_string())?;
        let path = self.state_path();
        write_atomic(&path, &data)?;
        inner.state_mod = mod_time(&path);
        Ok(())
    }

    /// Copies a source so network I/O can run without holding the lock.
    fn snapshot(&self, id: &str) -> Result<Option<Source>> {
        Ok(self.read()?.sources.iter().find(|s| s.id == id).cloned())
    }

    fn state_path(&self) -> PathBuf {
        self.dir.join("library.json")
    }
}

impl Source {
    fn info(&self) -> SourceInfo {
        let location = match self.kind {
            Kind::M3uUrl => redact_url(&self.url),
            Kind::M3uFile => self.path.clone(),
            Kind::Xtream => Xtream {
                server: &self.server,
                username: "",
                password: "",
            }
            .location(),
        };
        SourceInfo {
            id: self.id.clone(),
            name: self.name.clone(),
            kind: self.kind,
            location,
            channel_count: self.channel_count,
            updated_at: self.updated_at,
            enabled: !self.disabled,
            expires_at: self.expires_at,
            account_status: self.account_status.clone(),
        }
    }
}

async fn load(s: &Source) -> Result<Loaded> {
    let entries = match s.kind {
        Kind::M3uUrl => playlist::m3u_from_url(&s.url).await?,
        // Up to 256 MB of blocking reads: keep them off the async workers.
        Kind::M3uFile => {
            let path = s.path.clone();
            tauri::async_runtime::spawn_blocking(move || playlist::m3u_from_file(&path))
                .await
                .map_err(|e| e.to_string())??
        }
        Kind::Xtream => {
            if s.username.is_empty() || s.password.is_empty() {
                return Err("credentials_required".into());
            }
            let x = Xtream {
                server: &s.server,
                username: &s.username,
                password: &s.password,
            };
            let entries = x.live_channels().await?;
            // Expiry is nice to have: a panel that hides it must not block loading.
            let account: Account = x.account_info().await.unwrap_or_default();
            return Ok(Loaded { entries, account });
        }
    };
    Ok(Loaded {
        entries,
        account: Account::default(),
    })
}

fn position(inner: &Inner, id: &str) -> Option<usize> {
    inner.sources.iter().position(|s| s.id == id)
}

fn find_channel<'a>(inner: &'a Inner, channel_id: &str) -> Option<&'a StoredChannel> {
    inner
        .channels
        .get(source_of(channel_id))
        .and_then(|chs| chs.iter().find(|c| c.id == channel_id))
}

fn source_of(channel_id: &str) -> &str {
    channel_id.split_once('-').map_or(channel_id, |(s, _)| s)
}

/// "<sourceID>-<fnv64a(url)>". Must match the Go build byte for byte so
/// favourites carry over between the two apps.
pub fn channel_id(source_id: &str, url: &str) -> String {
    let mut h: u64 = 0xcbf2_9ce4_8422_2325;
    for b in url.bytes() {
        h ^= u64::from(b);
        h = h.wrapping_mul(0x0000_0100_0000_01b3);
    }
    format!("{source_id}-{h:016x}")
}

fn channels_path(dir: &Path, id: &str) -> PathBuf {
    dir.join("channels").join(format!("{id}.json"))
}

/// Drops credentials and the query: provider M3U links usually look like
/// get.php?username=X&password=Y, and the UI only needs to recognise the host.
fn redact_url(raw: &str) -> String {
    let Ok(mut u) = reqwest::Url::parse(raw) else {
        return String::new();
    };
    let _ = u.set_username("");
    let _ = u.set_password(None);
    u.set_query(None);
    u.set_fragment(None);
    u.into()
}

fn valid_id(id: &str) -> bool {
    id.len() == 12
        && id
            .bytes()
            .all(|b| b.is_ascii_digit() || (b'a'..=b'f').contains(&b))
}

fn new_id() -> Result<String> {
    let mut b = [0u8; 6];
    getrandom::fill(&mut b).map_err(|e| e.to_string())?;
    Ok(b.iter().map(|x| format!("{x:02x}")).collect())
}

fn now() -> i64 {
    SystemTime::now()
        .duration_since(UNIX_EPOCH)
        .map_or(0, |d| d.as_secs() as i64)
}

fn set_flag(set: &mut HashSet<String>, key: &str, on: bool) {
    if on {
        set.insert(key.to_string());
    } else {
        set.remove(key);
    }
}

/// Stable file contents across saves and across builds.
fn sorted(set: &HashSet<String>) -> Vec<String> {
    let mut v: Vec<String> = set.iter().cloned().collect();
    v.sort();
    v
}

fn is_false(b: &bool) -> bool {
    !b
}

fn is_zero(n: &i64) -> bool {
    *n == 0
}

fn mod_time(path: &Path) -> Option<SystemTime> {
    fs::metadata(path).and_then(|m| m.modified()).ok()
}

/// Temp file + rename so a crash never leaves a half-written library.
/// Files are 0600: library.json holds Xtream passwords.
fn write_atomic(path: &Path, data: &[u8]) -> Result<()> {
    let tmp = path.with_file_name(format!(".tmp-{}", new_id()?));
    let write = || -> io::Result<()> {
        let mut f = fs::OpenOptions::new()
            .write(true)
            .create_new(true)
            .mode(0o600)
            .open(&tmp)?;
        f.write_all(data)?;
        f.sync_all()?;
        fs::rename(&tmp, path)
    };
    write().map_err(|e| {
        let _ = fs::remove_file(&tmp);
        e.to_string()
    })
}
