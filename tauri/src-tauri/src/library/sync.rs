//! Both builds can run at once against the same files. Each keeps the
//! library in memory, so without this the last one to save would silently
//! drop what the other added. Before every read or write we compare
//! library.json's modification time with the one we last saw and reload if it
//! moved. It is last-writer-wins per operation, not a merge, which is enough
//! for one person clicking in two windows. Mirrors wails/internal/library/sync.go.

use std::{
    collections::HashMap,
    fs, io,
    sync::{RwLockReadGuard, RwLockWriteGuard},
    time::{Duration, SystemTime},
};

use super::{
    Inner, LOCKED, Library, Result, State, StoredChannel, channels_path, mod_time, valid_id,
    with_keys,
};

/// Leaves alone files the other app may be writing right now.
pub(super) const ORPHAN_GRACE: Duration = Duration::from_secs(60);

impl Library {
    /// Read access after reloading anything the other app changed.
    pub(super) fn read(&self) -> Result<RwLockReadGuard<'_, Inner>> {
        let stale = {
            let inner = self.inner.read().map_err(|_| LOCKED)?;
            mod_time(&self.state_path()) != inner.state_mod
        };
        if stale {
            drop(self.write()?);
        }
        self.inner.read().map_err(|_| LOCKED.into())
    }

    /// Write access after reloading anything the other app changed. A file
    /// that fails to parse mid-write by the other app is ignored until next time.
    pub(super) fn write(&self) -> Result<RwLockWriteGuard<'_, Inner>> {
        let mut inner = self.inner.write().map_err(|_| LOCKED)?;
        if mod_time(&self.state_path()) != inner.state_mod {
            let _ = self.reload_locked(&mut inner);
        }
        Ok(inner)
    }

    /// Reads library.json and any channel cache that changed.
    pub(super) fn reload_locked(&self, inner: &mut Inner) -> Result<()> {
        let st: State = match fs::read(self.state_path()) {
            Ok(data) => {
                serde_json::from_slice(&data).map_err(|e| format!("library_corrupt: {e}"))?
            }
            Err(e) if e.kind() == io::ErrorKind::NotFound => State::default(),
            Err(e) => return Err(e.to_string()),
        };
        // IDs become file names; refuse anything a hand-edited file could use to escape channels/.
        if let Some(bad) = st.sources.iter().find(|s| !valid_id(&s.id)) {
            return Err(format!("library_corrupt: invalid source id {:?}", bad.id));
        }

        let mut channels = HashMap::with_capacity(st.sources.len());
        let mut chan_mod = HashMap::with_capacity(st.sources.len());
        for s in &st.sources {
            let path = channels_path(&self.dir, &s.id);
            let modified = mod_time(&path);
            let unchanged = inner.chan_mod.get(&s.id) == Some(&modified);
            let chs: Vec<StoredChannel> = match inner.channels.remove(&s.id) {
                Some(cached) if unchanged => cached,
                _ => match fs::read(&path) {
                    Ok(data) => with_keys(
                        serde_json::from_slice(&data)
                            .map_err(|e| format!("channels_corrupt: {:?}: {e}", s.name))?,
                    ),
                    Err(e) if e.kind() == io::ErrorKind::NotFound => Vec::new(),
                    Err(e) => return Err(e.to_string()),
                },
            };
            channels.insert(s.id.clone(), chs);
            chan_mod.insert(s.id.clone(), modified);
        }

        inner.sources = st.sources;
        inner.favorites = st.favorites.into_iter().collect();
        inner.hidden_channels = st.hidden_channels.into_iter().collect();
        inner.hidden_groups = st.hidden_groups.into_iter().collect();
        inner.history = st.history;
        inner.settings = st.settings;
        inner.channels = channels;
        inner.chan_gen += 1;
        inner.chan_mod = chan_mod;
        inner.state_mod = mod_time(&self.state_path());
        Ok(())
    }

    /// Deletes channel caches no source points at (left behind when both
    /// apps used to overwrite each other) and stale temp files.
    pub(super) fn remove_orphans(&self, inner: &Inner) {
        let known: std::collections::HashSet<String> = inner
            .sources
            .iter()
            .map(|s| format!("{}.json", s.id))
            .collect();
        let cutoff = SystemTime::now() - ORPHAN_GRACE;
        let channels_dir = self.dir.join("channels");
        for dir in [self.dir.clone(), channels_dir.clone()] {
            let Ok(entries) = fs::read_dir(&dir) else {
                continue;
            };
            for e in entries.flatten() {
                let name = e.file_name().to_string_lossy().into_owned();
                let orphan = name.starts_with(".tmp-")
                    || (dir == channels_dir && name.ends_with(".json") && !known.contains(&name));
                let old = e
                    .metadata()
                    .and_then(|m| m.modified())
                    .is_ok_and(|t| t < cutoff);
                if orphan && old && e.file_type().is_ok_and(|t| t.is_file()) {
                    let _ = fs::remove_file(e.path());
                }
            }
        }
    }
}
