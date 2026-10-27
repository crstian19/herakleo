//! Hidden channels and groups, watch history and favourites backup.
//! Mirrors wails/internal/library/curation.go.

use std::{collections::HashMap, fs, io::Read, path::Path};

use serde::{Deserialize, Serialize};

use super::{HistoryEntry, Library, Result, now, set_flag, sorted};

/// Keeps the Recent view short and library.json small.
const MAX_HISTORY: usize = 50;
const BACKUP_FORMAT: &str = "herakleo-favorites";
/// Bounds an imported file; real backups are a few KB.
const MAX_BACKUP_BYTES: u64 = 8 << 20;

/// A favourite named by list, channel and group instead of IDs, so a backup
/// still matches after a list is deleted and added again (new source ID) or
/// moved to another machine. Stream URLs are left out: they carry passwords.
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct FavoriteRef {
    source: String,
    channel: String,
    #[serde(default)]
    group: String,
}

/// File format of an exported favourites list.
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct FavoritesBackup {
    #[serde(default)]
    format: String,
    #[serde(default)]
    version: u32,
    #[serde(default)]
    favorites: Vec<FavoriteRef>,
}

impl Library {
    /// Hides or unhides one channel everywhere but the hidden view.
    pub fn set_channel_hidden(&self, channel_id: &str, hidden: bool) -> Result<()> {
        let mut inner = self.write()?;
        set_flag(&mut inner.hidden_channels, channel_id, hidden);
        self.save_state(&mut inner)
    }

    /// Hides or unhides a group by name across all sources: lists from the
    /// same provider repeat group names, and that is what users expect.
    pub fn set_group_hidden(&self, name: &str, hidden: bool) -> Result<()> {
        let mut inner = self.write()?;
        set_flag(&mut inner.hidden_groups, name, hidden);
        self.save_state(&mut inner)
    }

    pub fn hidden_groups(&self) -> Result<Vec<String>> {
        Ok(sorted(&self.read()?.hidden_groups))
    }

    /// Records a played channel at the top of the history.
    pub fn add_history(&self, channel_id: &str) -> Result<()> {
        let mut inner = self.write()?;
        inner.history.retain(|h| h.id != channel_id);
        inner.history.insert(
            0,
            HistoryEntry {
                id: channel_id.to_string(),
                at: now(),
            },
        );
        inner.history.truncate(MAX_HISTORY);
        self.save_state(&mut inner)
    }

    pub fn clear_history(&self) -> Result<()> {
        let mut inner = self.write()?;
        inner.history.clear();
        self.save_state(&mut inner)
    }

    /// Every favourite whose channel still exists.
    pub fn export_favorites(&self) -> Result<FavoritesBackup> {
        let inner = self.read()?;
        let mut favorites = Vec::new();
        for s in &inner.sources {
            for c in inner.channels.get(&s.id).into_iter().flatten() {
                if inner.favorites.contains(&c.id) {
                    favorites.push(FavoriteRef {
                        source: s.name.clone(),
                        channel: c.name.clone(),
                        group: c.group.clone(),
                    });
                }
            }
        }
        Ok(FavoritesBackup {
            format: BACKUP_FORMAT.into(),
            version: 1,
            favorites,
        })
    }

    /// Marks the channels a backup names and returns how many it found. A
    /// list is matched by name (case-insensitive); inside it a channel by name
    /// and group, else by name alone.
    pub fn import_favorites(&self, b: FavoritesBackup) -> Result<usize> {
        if b.format != BACKUP_FORMAT {
            return Err("backup_invalid".into());
        }
        let mut inner = self.write()?;
        let mut by_full: HashMap<(String, &str, &str), String> = HashMap::new();
        let mut by_name: HashMap<(String, &str), String> = HashMap::new();
        for s in &inner.sources {
            let src = s.name.to_lowercase();
            for c in inner.channels.get(&s.id).into_iter().flatten() {
                by_full
                    .entry((src.clone(), &c.name, &c.group))
                    .or_insert_with(|| c.id.clone());
                by_name
                    .entry((src.clone(), &c.name))
                    .or_insert_with(|| c.id.clone());
            }
        }

        let found: Vec<String> = b
            .favorites
            .iter()
            .filter_map(|f| {
                let src = f.source.to_lowercase();
                by_full
                    .get(&(src.clone(), f.channel.as_str(), f.group.as_str()))
                    .or_else(|| by_name.get(&(src, f.channel.as_str())))
                    .cloned()
            })
            .collect();
        if found.is_empty() {
            return Ok(0);
        }
        let n = found.len();
        inner.favorites.extend(found);
        self.save_state(&mut inner)?;
        Ok(n)
    }
}

/// Saves an exported favourites list as indented JSON.
pub fn write_backup(path: &Path, b: &FavoritesBackup) -> Result<()> {
    let data = serde_json::to_vec_pretty(b).map_err(|e| e.to_string())?;
    fs::write(path, data).map_err(|e| e.to_string())
}

/// Loads a favourites list chosen by the user.
pub fn read_backup(path: &Path) -> Result<FavoritesBackup> {
    let mut data = Vec::new();
    fs::File::open(path)
        .and_then(|f| f.take(MAX_BACKUP_BYTES).read_to_end(&mut data))
        .map_err(|e| e.to_string())?;
    serde_json::from_slice(&data).map_err(|_| "backup_invalid".into())
}
