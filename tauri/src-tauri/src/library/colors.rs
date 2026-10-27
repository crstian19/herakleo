//! Logo accent colour cache. Mirrors wails/internal/library/colors.go.

use std::{collections::HashMap, fs};

use super::{Library, write_atomic};

const MAX_COLOR_BATCH: usize = 64;
/// Parallel logo downloads per batch; the webview fetches the same logos.
const COLOR_WORKERS: usize = 8;

impl Library {
    /// "#rrggbb" (or "" when unusable) for each logo URL, cached in memory
    /// and in colors.json (shared with the Go build). Failures are cached in
    /// memory only, so a flaky host is retried on the next launch.
    pub async fn logo_colors(&self, mut urls: Vec<String>) -> HashMap<String, String> {
        use futures_util::{StreamExt, stream};

        urls.truncate(MAX_COLOR_BATCH);
        let mut out = HashMap::with_capacity(urls.len());
        let mut todo = Vec::new();
        {
            let Ok(mut cache) = self.colors.lock() else {
                return out;
            };
            let cache = cache.get_or_insert_with(|| self.load_colors());
            for u in urls {
                if let Some(c) = cache.get(&u) {
                    out.insert(u, c.clone());
                } else if !out.contains_key(&u) {
                    out.insert(u.clone(), String::new());
                    todo.push(u);
                }
            }
        }
        if todo.is_empty() {
            return out;
        }

        let fetched: Vec<(String, String)> = stream::iter(todo)
            .map(|u| async move {
                let c = crate::logo_color::from_url(&u).await.unwrap_or_default();
                (u, c)
            })
            .buffer_unordered(COLOR_WORKERS)
            .collect()
            .await;

        let Ok(mut cache) = self.colors.lock() else {
            return out;
        };
        let cache = cache.get_or_insert_with(HashMap::new);
        let mut changed = false;
        for (u, c) in fetched {
            changed |= !c.is_empty();
            cache.insert(u.clone(), c.clone());
            out.insert(u, c);
        }
        if changed {
            // A lost cache write only costs a re-download.
            let ok: HashMap<&String, &String> =
                cache.iter().filter(|(_, c)| !c.is_empty()).collect();
            if let Ok(data) = serde_json::to_vec(&ok) {
                let _ = write_atomic(&self.dir.join("colors.json"), &data);
            }
        }
        out
    }

    fn load_colors(&self) -> HashMap<String, String> {
        fs::read(self.dir.join("colors.json"))
            .ok()
            .and_then(|d| serde_json::from_slice(&d).ok())
            .unwrap_or_default()
    }
}
