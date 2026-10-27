//! Tauri commands; each one mirrors a bound method on the Wails `App`.
//!
//! Every command is async: Tauri v2 runs sync commands on the main thread,
//! where a 100k-channel scan or waiting on the library lock would freeze the
//! UI. Wails already runs each call on its own goroutine, so this keeps the
//! comparison fair.

mod library;
mod logo_color;
mod player;
mod playlist;

use library::{Group, Library, NewSource, Page, Query, SettingsPatch, SourceInfo};
use player::Player;
use tauri::{Manager, RunEvent, State};
use tauri_plugin_dialog::DialogExt;

type Result<T> = std::result::Result<T, String>;

#[tauri::command(async)]
fn list_sources(lib: State<'_, Library>) -> Result<Vec<SourceInfo>> {
    lib.sources()
}

#[tauri::command]
async fn add_source(lib: State<'_, Library>, req: NewSource) -> Result<SourceInfo> {
    lib.add(req).await
}

#[tauri::command]
async fn refresh_source(lib: State<'_, Library>, id: String) -> Result<SourceInfo> {
    lib.refresh(&id).await
}

#[tauri::command]
async fn update_source(lib: State<'_, Library>, id: String, req: NewSource) -> Result<SourceInfo> {
    lib.update(&id, req).await
}

#[tauri::command(async)]
fn remove_source(lib: State<'_, Library>, id: String) -> Result<()> {
    lib.remove(&id)
}

#[tauri::command(async)]
fn list_groups(lib: State<'_, Library>, source_id: String) -> Result<Vec<Group>> {
    lib.groups(&source_id)
}

#[tauri::command(async)]
fn list_channels(lib: State<'_, Library>, req: Query) -> Result<Page> {
    lib.channels(req)
}

#[tauri::command(async)]
fn set_favorite(lib: State<'_, Library>, channel_id: String, favorite: bool) -> Result<()> {
    lib.set_favorite(&channel_id, favorite)
}

#[tauri::command(async)]
fn set_source_enabled(lib: State<'_, Library>, id: String, enabled: bool) -> Result<SourceInfo> {
    lib.set_enabled(&id, enabled)
}

#[tauri::command(async)]
fn set_channel_hidden(lib: State<'_, Library>, channel_id: String, hidden: bool) -> Result<()> {
    lib.set_channel_hidden(&channel_id, hidden)
}

#[tauri::command(async)]
fn set_group_hidden(lib: State<'_, Library>, name: String, hidden: bool) -> Result<()> {
    lib.set_group_hidden(&name, hidden)
}

#[tauri::command(async)]
fn list_hidden_groups(lib: State<'_, Library>) -> Result<Vec<String>> {
    lib.hidden_groups()
}

#[tauri::command(async)]
fn clear_history(lib: State<'_, Library>) -> Result<()> {
    lib.clear_history()
}

/// Opens the channel and records it in the history; a failed history write
/// must not stop playback that already started.
#[tauri::command(async)]
fn play(lib: State<'_, Library>, player: State<'_, Player>, channel_id: String) -> Result<()> {
    let (name, url) = lib.stream_url(&channel_id)?;
    let mut opts = player_options(&lib.settings()?);
    if opts.validate().is_err() {
        opts = player::Options::default(); // a hand-edited library.json must not reach the player
    }
    player.play(&player_id(&lib)?, &name, &url, opts)?;
    let _ = lib.add_history(&channel_id);
    Ok(())
}

/// The shared settings plus what this machine can play with.
#[derive(serde::Serialize)]
#[serde(rename_all = "camelCase")]
struct SettingsView {
    player: String,
    players: Vec<player::Info>,
    refresh_on_start: bool,
    software_decoding: bool,
    cache_secs: u32,
    volume: u32,
    cache_choices: [u32; 5],
}

#[tauri::command(async)]
fn get_settings(lib: State<'_, Library>) -> Result<SettingsView> {
    settings_view(&lib)
}

/// Applies a partial change; the player must be on the allowlist.
#[tauri::command(async)]
fn update_settings(lib: State<'_, Library>, patch: SettingsPatch) -> Result<SettingsView> {
    if let Some(id) = &patch.player
        && !player::known(id)
    {
        return Err(format!("unknown_player: {id}"));
    }
    let mut next = player_options(&lib.settings()?);
    next.cache_secs = patch.cache_secs.unwrap_or(next.cache_secs);
    next.volume = patch.volume.unwrap_or(next.volume);
    next.validate()?;
    lib.update_settings(patch)?;
    settings_view(&lib)
}

fn settings_view(lib: &Library) -> Result<SettingsView> {
    let s = lib.settings()?;
    Ok(SettingsView {
        player: player_id(lib)?,
        players: player::available(),
        refresh_on_start: s.refresh_on_start,
        software_decoding: s.software_decoding,
        cache_secs: s.cache_secs,
        volume: s.volume,
        cache_choices: player::CACHE_CHOICES,
    })
}

fn player_options(s: &library::Settings) -> player::Options {
    player::Options {
        software_decoding: s.software_decoding,
        cache_secs: s.cache_secs,
        volume: s.volume,
    }
}

fn player_id(lib: &Library) -> Result<String> {
    let id = lib.settings()?.player;
    Ok(if player::known(&id) {
        id
    } else {
        player::DEFAULT_ID.into()
    })
}

/// Asks where to save and returns the path, or None if cancelled.
#[tauri::command]
async fn export_favorites(
    app: tauri::AppHandle,
    lib: State<'_, Library>,
    title: String,
) -> Result<Option<String>> {
    let backup = lib.export_favorites()?;
    let picked = tauri::async_runtime::spawn_blocking(move || {
        app.dialog()
            .file()
            .set_title(title)
            .set_file_name("herakleo-favorites.json")
            .add_filter("JSON", &["json"])
            .blocking_save_file()
    })
    .await
    .map_err(|e| e.to_string())?;
    let Some(path) = picked else { return Ok(None) };
    let path = path.into_path().map_err(|e| e.to_string())?;
    library::write_backup(&path, &backup)?;
    Ok(Some(path.to_string_lossy().into_owned()))
}

/// Asks for a backup and returns how many favourites matched, or None if cancelled.
#[tauri::command]
async fn import_favorites(
    app: tauri::AppHandle,
    lib: State<'_, Library>,
    title: String,
) -> Result<Option<usize>> {
    let picked = tauri::async_runtime::spawn_blocking(move || {
        app.dialog()
            .file()
            .set_title(title)
            .add_filter("JSON", &["json"])
            .blocking_pick_file()
    })
    .await
    .map_err(|e| e.to_string())?;
    let Some(path) = picked else { return Ok(None) };
    let backup = library::read_backup(&path.into_path().map_err(|e| e.to_string())?)?;
    lib.import_favorites(backup).map(Some)
}

#[tauri::command]
async fn logo_colors(
    lib: State<'_, Library>,
    urls: Vec<String>,
) -> Result<std::collections::HashMap<String, String>> {
    Ok(lib.logo_colors(urls).await)
}

/// Describes the machine for the in-app performance report (Settings →
/// Performance), so a tester can send it without a terminal. Only
/// display/renderer environment variables are included: never secrets.
/// Same contract as Go's Diagnostics.
#[tauri::command(async)]
fn diagnostics() -> std::collections::HashMap<String, String> {
    let mut out = std::collections::HashMap::from([
        (
            "webview".to_string(),
            "webkit2gtk-4.1 (GTK 3, Tauri 2)".to_string(),
        ),
        (
            "session".into(),
            std::env::var("XDG_SESSION_TYPE").unwrap_or_default(),
        ),
        (
            "desktop".into(),
            std::env::var("XDG_CURRENT_DESKTOP").unwrap_or_default(),
        ),
        ("os".into(), os_name()),
    ]);
    const PREFIXES: [&str; 6] = ["WEBKIT_", "GSK_", "GDK_", "MESA_", "__GL_", "LIBGL_"];
    for (k, v) in std::env::vars() {
        if PREFIXES.iter().any(|p| k.starts_with(p)) {
            out.insert(format!("env.{k}"), v);
        }
    }
    out
}

fn os_name() -> String {
    std::fs::read_to_string("/etc/os-release")
        .ok()
        .and_then(|s| {
            s.lines().find_map(|l| {
                l.strip_prefix("PRETTY_NAME=")
                    .map(|v| v.trim_matches('"').to_string())
            })
        })
        .unwrap_or_default()
}

/// The only URLs the UI may open in the system browser: it sends an ID, never
/// a URL, so a webview bug cannot open arbitrary links. Same list as Go's
/// projectLinks. Donation links match https://blog.crstian.me/support/.
const PROJECT_LINKS: [(&str, &str); 4] = [
    ("github", "https://github.com/crstian19/herakleo"),
    ("sponsors", "https://github.com/sponsors/crstian19"),
    ("kofi", "https://ko-fi.com/crstian"),
    ("paypal", "https://paypal.me/CrstianG"),
];

/// Opens one of PROJECT_LINKS in the default browser. Called from Rust, so
/// the webview needs no opener permission.
#[tauri::command(async)]
fn open_link(app: tauri::AppHandle, id: String) -> Result<()> {
    let url = PROJECT_LINKS
        .iter()
        .find(|(k, _)| *k == id)
        .map(|(_, u)| *u)
        .ok_or_else(|| format!("unknown_link: {id}"))?;
    open_url(&app, url)
}

/// Opens `url` through a GDK launch context, which hands the browser an XDG
/// activation token on Wayland (a startup ID on X11) so the compositor may bring
/// it to the front; plain xdg-open passes none and the tab opens in the
/// background. Sync commands run on the main thread, which GDK needs.
#[cfg(target_os = "linux")]
fn open_url(_app: &tauri::AppHandle, url: &str) -> Result<()> {
    let ctx = gtk::gdk::Display::default().and_then(|d| d.app_launch_context());
    gtk::gio::AppInfo::launch_default_for_uri(url, ctx.as_ref()).map_err(|e| e.to_string())
}

#[cfg(not(target_os = "linux"))]
fn open_url(app: &tauri::AppHandle, url: &str) -> Result<()> {
    use tauri_plugin_opener::OpenerExt;
    app.opener()
        .open_url(url, None::<&str>)
        .map_err(|e| e.to_string())
}

/// Names the file an unattended benchmark run writes to (just bench). The
/// path comes from the environment, never from the UI.
const BENCH_ENV: &str = "HERAKLEO_BENCH_OUT";

/// "" outside a benchmark run, otherwise HERAKLEO_BENCH_MODE ("full" by
/// default; "wheel[:variant]" for scripts/profile-scroll.sh). Same contract
/// as Go's BenchMode.
#[tauri::command(async)]
fn bench_mode() -> String {
    if std::env::var_os(BENCH_ENV).is_none() {
        return String::new();
    }
    std::env::var("HERAKLEO_BENCH_MODE")
        .ok()
        .filter(|m| !m.is_empty())
        .unwrap_or_else(|| "full".into())
}

/// Writes the frontend's measurements and quits. Same contract as Go's BenchReport.
#[tauri::command(async)]
fn bench_report(app: tauri::AppHandle, report: String) -> Result<()> {
    let path = std::env::var_os(BENCH_ENV).ok_or("bench_disabled")?;
    if report.len() > 1 << 20 {
        return Err("bench_report_too_large".into());
    }
    std::fs::write(path, report).map_err(|e| e.to_string())?;
    app.exit(0);
    Ok(())
}

/// Returns None when the dialog is cancelled. Dialog titles come from the UI so
/// they follow its language. The blocking dialog runs off the async runtime's
/// workers; sync commands run on the main thread in Tauri v2, where blocking
/// would freeze the UI.
#[tauri::command]
async fn pick_playlist_file(app: tauri::AppHandle, title: String) -> Result<Option<String>> {
    let picked = tauri::async_runtime::spawn_blocking(move || {
        app.dialog()
            .file()
            .set_title(title)
            .add_filter("M3U", &["m3u", "m3u8"])
            .blocking_pick_file()
    })
    .await
    .map_err(|e| e.to_string())?;

    picked
        .map(|p| {
            p.into_path()
                .map(|p| p.to_string_lossy().into_owned())
                .map_err(|e| e.to_string())
        })
        .transpose()
}

pub fn run() {
    tauri::Builder::default()
        .plugin(tauri_plugin_dialog::init())
        .plugin(tauri_plugin_opener::init())
        .setup(|app| {
            // Same directory as the Wails build: ~/.config/herakleo.
            let dir = app.path().config_dir()?.join("herakleo");
            app.manage(Library::open(dir)?);
            app.manage(Player::default());
            Ok(())
        })
        .invoke_handler(tauri::generate_handler![
            list_sources,
            add_source,
            refresh_source,
            update_source,
            remove_source,
            list_groups,
            list_channels,
            set_favorite,
            set_source_enabled,
            set_channel_hidden,
            set_group_hidden,
            list_hidden_groups,
            clear_history,
            play,
            get_settings,
            update_settings,
            export_favorites,
            import_favorites,
            pick_playlist_file,
            bench_mode,
            diagnostics,
            open_link,
            bench_report,
            logo_colors,
        ])
        .build(tauri::generate_context!())
        .expect("error while building tauri application")
        .run(|app, event| {
            if let RunEvent::Exit = event {
                app.state::<Player>().stop();
            }
        });
}
