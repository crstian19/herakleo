//! Channel queries: views, sorting, hidden items. Mirrors
//! wails/internal/library/query.go.

use std::collections::HashMap;

use serde::{Deserialize, Serialize};

use super::{
    Inner, LOCKED, Library, Result, SortCache, Source, StoredChannel, find_channel, source_of,
};

/// A channel as the UI sees it (no stream URL).
#[derive(Debug, Clone, Serialize)]
#[serde(rename_all = "camelCase")]
pub struct Channel {
    pub(super) id: String,
    source_id: String,
    pub(super) name: String,
    pub(super) group: String,
    logo: String,
    favorite: bool,
    hidden: bool,
}

#[derive(Debug, Serialize)]
pub struct Group {
    pub(super) name: String,
    pub(super) count: usize,
}

/// Empty fields mean "any". `view`: "" | "favorites" | "history" (newest
/// first, ignores sort) | "hidden". `sort`: "" (provider order) | "az" | "za".
#[derive(Debug, Default, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct Query {
    #[serde(default)]
    pub(super) source_id: String,
    #[serde(default)]
    pub(super) group: String,
    #[serde(default)]
    pub(super) search: String,
    #[serde(default)]
    pub(super) view: String,
    #[serde(default)]
    pub(super) sort: String,
    #[serde(default)]
    pub(super) offset: usize,
    #[serde(default)]
    pub(super) limit: usize,
}

#[derive(Debug, Serialize)]
pub struct Page {
    pub(super) items: Vec<Channel>,
    pub(super) total: usize,
}

const MAX_LIMIT: usize = 500;

impl Library {
    /// Linear scan: even 100k channels filter in a few milliseconds, which
    /// beats keeping an index in sync. Disabled sources are always skipped;
    /// hidden channels and groups only show in the hidden view.
    pub fn channels(&self, q: Query) -> Result<Page> {
        let inner = self.read()?;
        let limit = if q.limit == 0 || q.limit > MAX_LIMIT {
            MAX_LIMIT
        } else {
            q.limit
        };
        let needle = q.search.trim().to_lowercase();

        let matches = |s: &Source, c: &StoredChannel| -> bool {
            if s.disabled || (!q.source_id.is_empty() && s.id != q.source_id) {
                return false;
            }
            let hidden_ch = inner.hidden_channels.contains(&c.id);
            let hidden = hidden_ch || inner.hidden_groups.contains(&c.group);
            let visible = match q.view.as_str() {
                "hidden" => hidden_ch,
                "favorites" => !hidden && inner.favorites.contains(&c.id),
                _ => !hidden,
            };
            visible
                && (q.group.is_empty() || c.group == q.group)
                && (needle.is_empty() || c.lower.contains(&needle))
        };
        let view = |s: &Source, c: &StoredChannel| Channel {
            id: c.id.clone(),
            source_id: s.id.clone(),
            name: c.name.clone(),
            group: c.group.clone(),
            logo: c.logo.clone(),
            favorite: inner.favorites.contains(&c.id),
            hidden: inner.hidden_channels.contains(&c.id),
        };

        let mut all: Vec<Channel> = Vec::new();
        if q.view == "history" {
            for h in &inner.history {
                let src = inner.sources.iter().find(|s| s.id == source_of(&h.id));
                if let (Some(s), Some(c)) = (src, find_channel(&inner, &h.id))
                    && matches(s, c)
                {
                    all.push(view(s, c));
                }
            }
        } else {
            // Provider order walks the playlists; A→Z/Z→A walk the cached
            // sorted order. Either way nothing is sorted per query and only
            // the page's channels are copied out.
            let mut page = Page {
                items: Vec::new(),
                total: 0,
            };
            let mut add = |s: &Source, c: &StoredChannel| {
                if !matches(s, c) {
                    return;
                }
                if page.total >= q.offset && page.items.len() < limit {
                    page.items.push(view(s, c));
                }
                page.total += 1;
            };
            if q.sort == "az" || q.sort == "za" {
                let mut cache = self.sorted.lock().map_err(|_| LOCKED)?;
                let refs = sorted_refs(&inner, &mut cache);
                let mut visit = |&(si, ci): &(usize, usize)| {
                    let s = &inner.sources[si];
                    add(s, &inner.channels[&s.id][ci]);
                };
                if q.sort == "az" {
                    refs.iter().for_each(&mut visit);
                } else {
                    refs.iter().rev().for_each(&mut visit);
                }
            } else {
                for s in &inner.sources {
                    for c in channels_of(&inner, &s.id) {
                        add(s, c);
                    }
                }
            }
            return Ok(page);
        }

        let total = all.len();
        let items = all.into_iter().skip(q.offset).take(limit).collect();
        Ok(Page { items, total })
    }

    /// Visible groups in first-seen order, for one source or all of them,
    /// counting only visible channels.
    pub fn groups(&self, source_id: &str) -> Result<Vec<Group>> {
        let inner = self.read()?;
        let mut index: HashMap<&str, usize> = HashMap::new();
        let mut groups: Vec<Group> = Vec::new();
        for s in &inner.sources {
            if s.disabled || (!source_id.is_empty() && s.id != source_id) {
                continue;
            }
            for c in channels_of(&inner, &s.id) {
                if inner.hidden_groups.contains(&c.group) || inner.hidden_channels.contains(&c.id) {
                    continue;
                }
                let i = *index.entry(&c.group).or_insert_with(|| {
                    groups.push(Group {
                        name: c.group.clone(),
                        count: 0,
                    });
                    groups.len() - 1
                });
                groups[i].count += 1;
            }
        }
        Ok(groups)
    }
}

fn channels_of<'a>(inner: &'a Inner, source_id: &str) -> impl Iterator<Item = &'a StoredChannel> {
    inner.channels.get(source_id).into_iter().flatten()
}

/// Every channel in A→Z order, rebuilt only when `chan_gen` moves. Sorting
/// 65k names used to run on every page of the infinite scroll; now it runs
/// once per change of the channel lists. Indices stay valid while
/// `chan_gen` is unchanged, since every change to sources' channels bumps it.
fn sorted_refs<'a>(inner: &Inner, cache: &'a mut Option<SortCache>) -> &'a [(usize, usize)] {
    if cache.as_ref().is_none_or(|(g, _)| *g != inner.chan_gen) {
        let mut refs: Vec<(usize, usize)> = inner
            .sources
            .iter()
            .enumerate()
            .flat_map(|(si, s)| (0..channels_of(inner, &s.id).count()).map(move |ci| (si, ci)))
            .collect();
        // Case-insensitive, ties by exact name then ID: a total order,
        // identical in the Go build. Z→A is this order reversed.
        let ch = |&(si, ci): &(usize, usize)| &inner.channels[&inner.sources[si].id][ci];
        refs.sort_unstable_by(|a, b| {
            let (a, b) = (ch(a), ch(b));
            (&a.lower, &a.name, &a.id).cmp(&(&b.lower, &b.name, &b.id))
        });
        *cache = Some((inner.chan_gen, refs));
    }
    &cache.as_ref().expect("filled above").1
}
