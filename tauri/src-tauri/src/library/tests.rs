//! Same scenarios as wails/internal/library/*_test.go.

use std::{
    io::{BufRead, BufReader, Write},
    net::TcpListener,
    path::PathBuf,
    thread,
    time::Duration,
};

use super::{curation::FavoritesBackup, *};

/// Minimal HTTP/1.1 provider: M3U lists and an Xtream panel.
fn fake_provider() -> String {
    let listener = TcpListener::bind("127.0.0.1:0").unwrap();
    let addr = format!("http://{}", listener.local_addr().unwrap());
    thread::spawn(move || {
        for stream in listener.incoming().flatten() {
            let mut line = String::new();
            BufReader::new(&stream).read_line(&mut line).unwrap();
            let target = line.split_whitespace().nth(1).unwrap_or("");
            let body = if target == "/list.m3u" {
                "#EXTM3U\n#EXTINF:-1 group-title=\"News\",BBC\nhttp://s/bbc\n#EXTINF:-1,CNN\nhttp://s/cnn\n"
            } else if target == "/big.m3u" {
                "#EXTM3U\n#EXTINF:-1 group-title=\"News\",BBC\nhttp://s/bbc\n\
                 #EXTINF:-1 group-title=\"Sports\",Zeta\nhttp://s/zeta\n\
                 #EXTINF:-1 group-title=\"News\",cnn\nhttp://s/cnn\n\
                 #EXTINF:-1 group-title=\"Sports\",antena 3\nhttp://s/a3\n"
            } else if !target.contains("password=secret") {
                r#"{"user_info":{"auth":0}}"#
            } else if !target.contains("action=") {
                r#"{"user_info":{"auth":1,"status":"Active","exp_date":"1893456000"}}"#
            } else if target.contains("get_live_categories") {
                r#"[{"category_id":"7","category_name":"Sports"}]"#
            } else {
                r#"[{"name":"ESPN","stream_id":42,"category_id":"7"},{"name":"no id","stream_id":null}]"#
            };
            let mut s = &stream;
            let _ = write!(
                s,
                "HTTP/1.1 200 OK\r\nContent-Length: {}\r\nConnection: close\r\n\r\n{body}",
                body.len()
            );
        }
    });
    addr
}

fn src(kind: Kind, name: &str) -> NewSource {
    NewSource {
        kind,
        name: name.into(),
        url: String::new(),
        path: String::new(),
        server: String::new(),
        username: String::new(),
        password: String::new(),
    }
}

fn temp_dir() -> PathBuf {
    std::env::temp_dir().join(format!("herakleo-test-{}", new_id().unwrap()))
}

fn block<T>(f: impl Future<Output = T>) -> T {
    tauri::async_runtime::block_on(f)
}

fn q() -> Query {
    Query::default()
}

fn names(p: &Page) -> Vec<&str> {
    p.items.iter().map(|c| c.name.as_str()).collect()
}

/// A library with one M3U source of four channels in two groups.
fn test_library(dir: &Path) -> (Library, SourceInfo) {
    let srv = fake_provider();
    let lib = Library::open(dir.to_path_buf()).unwrap();
    let s = block(lib.add(NewSource {
        url: format!("{srv}/big.m3u"),
        ..src(Kind::M3uUrl, "List")
    }))
    .unwrap();
    (lib, s)
}

#[test]
fn channel_id_matches_go_fnv64a() {
    assert_eq!(channel_id("abc", ""), "abc-cbf29ce484222325");
    assert_eq!(
        channel_id("abc", "http://example.com/live/u/p/1.ts"),
        "abc-7a8bab84a10cac93"
    );
}

#[test]
fn add_m3u_xtream_update_and_reopen() {
    let srv = fake_provider();
    let dir = temp_dir();
    let lib = Library::open(dir.clone()).unwrap();

    block(async {
        let m3u = lib
            .add(NewSource {
                url: format!("{srv}/list.m3u"),
                ..src(Kind::M3uUrl, "list")
            })
            .await
            .unwrap();
        assert_eq!(m3u.channel_count, 2);

        let bad = NewSource {
            server: srv.clone(),
            username: "u".into(),
            password: "bad".into(),
            ..src(Kind::Xtream, "x")
        };
        assert!(lib.add(bad).await.is_err(), "bad credentials should fail");

        let x = lib
            .add(NewSource {
                server: format!("{srv}/"),
                username: "u".into(),
                password: "secret".into(),
                ..src(Kind::Xtream, "x")
            })
            .await
            .unwrap();
        assert_eq!(x.channel_count, 1);
        assert_eq!(
            (x.expires_at, x.account_status.as_str()),
            (1_893_456_000, "Active")
        );
        assert!(!x.location.contains("secret"));

        let page = lib
            .channels(Query {
                source_id: x.id.clone(),
                ..q()
            })
            .unwrap();
        assert_eq!(page.items[0].group, "Sports");
        let (_, stream) = lib.stream_url(&page.items[0].id).unwrap();
        assert_eq!(stream, format!("{srv}/live/u/secret/42.ts"));

        // Rename only: no download, credentials kept.
        let renamed = lib
            .update(
                &x.id,
                NewSource {
                    name: "renamed".into(),
                    ..src(Kind::Xtream, "")
                },
            )
            .await
            .unwrap();
        assert_eq!(
            (renamed.name.as_str(), renamed.channel_count),
            ("renamed", 1)
        );
        // Bad new password: rejected and nothing changes.
        let bad = NewSource {
            password: "bad".into(),
            ..src(Kind::Xtream, "")
        };
        assert!(lib.update(&x.id, bad).await.is_err());
        let (_, stream) = lib.stream_url(&page.items[0].id).unwrap();
        assert!(stream.contains("/secret/"));

        let again = Library::open(dir.clone()).unwrap();
        assert_eq!(again.channels(q()).unwrap().total, 3);
        again.remove(&m3u.id).unwrap();
        assert_eq!(again.channels(q()).unwrap().total, 1);
    });
    let _ = fs::remove_dir_all(dir);
}

/// The A→Z order is cached; changing the channel lists must invalidate it.
/// Same scenario as TestSortedCacheFollowsChanges in curation_test.go.
#[test]
fn sorted_cache_follows_changes() {
    let dir = temp_dir();
    let (lib, s) = test_library(&dir);
    let az = || {
        lib.channels(Query {
            sort: "az".into(),
            ..q()
        })
        .unwrap()
    };
    assert_eq!(az().total, 4);
    lib.remove(&s.id).unwrap();
    let after = az();
    assert_eq!((after.total, after.items.len()), (0, 0));
    let _ = fs::remove_dir_all(dir);
}

#[test]
fn sort_hide_and_groups() {
    let dir = temp_dir();
    let (lib, _) = test_library(&dir);

    let az = lib
        .channels(Query {
            sort: "az".into(),
            ..q()
        })
        .unwrap();
    assert_eq!(names(&az), ["antena 3", "BBC", "cnn", "Zeta"]);
    let za = lib
        .channels(Query {
            sort: "za".into(),
            limit: 2,
            offset: 1,
            ..q()
        })
        .unwrap();
    assert_eq!(names(&za), ["cnn", "BBC"]);

    let bbc = lib.channels(q()).unwrap().items[0].id.clone();
    lib.set_channel_hidden(&bbc, true).unwrap();
    lib.set_group_hidden("Sports", true).unwrap();
    assert_eq!(names(&lib.channels(q()).unwrap()), ["cnn"]);
    assert_eq!(
        names(
            &lib.channels(Query {
                view: "hidden".into(),
                ..q()
            })
            .unwrap()
        ),
        ["BBC"]
    );
    let groups = lib.groups("").unwrap();
    assert_eq!(
        (groups.len(), groups[0].name.as_str(), groups[0].count),
        (1, "News", 1)
    );
    assert_eq!(lib.hidden_groups().unwrap(), ["Sports"]);
    let _ = fs::remove_dir_all(dir);
}

#[test]
fn history_and_disable() {
    let dir = temp_dir();
    let (lib, s) = test_library(&dir);
    let all = lib.channels(q()).unwrap().items;
    for c in [&all[0], &all[1], &all[0]] {
        lib.add_history(&c.id).unwrap();
    }
    let history = lib
        .channels(Query {
            view: "history".into(),
            ..q()
        })
        .unwrap();
    assert_eq!(
        names(&history),
        [all[0].name.as_str(), all[1].name.as_str()]
    );

    lib.set_enabled(&s.id, false).unwrap();
    assert_eq!(lib.channels(q()).unwrap().total, 0);
    assert!(lib.groups("").unwrap().is_empty());
    lib.clear_history().unwrap();
    lib.set_enabled(&s.id, true).unwrap();
    assert_eq!(
        lib.channels(Query {
            view: "history".into(),
            ..q()
        })
        .unwrap()
        .total,
        0
    );
    let _ = fs::remove_dir_all(dir);
}

/// Two Library values on one dir stand in for the Tauri and Wails apps.
#[test]
fn two_apps_do_not_overwrite_each_other() {
    let dir = temp_dir();
    let (a, s) = test_library(&dir);
    let b = Library::open(dir.clone()).unwrap();
    thread::sleep(Duration::from_millis(10)); // distinct mtimes on coarse filesystems

    let srv = fake_provider();
    block(b.add(NewSource {
        url: format!("{srv}/list.m3u"),
        ..src(Kind::M3uUrl, "Otra")
    }))
    .unwrap();
    thread::sleep(Duration::from_millis(10));
    // a saves after b: without the mtime check this dropped b's list.
    let first = a
        .channels(Query {
            source_id: s.id.clone(),
            ..q()
        })
        .unwrap()
        .items[0]
        .id
        .clone();
    a.set_favorite(&first, true).unwrap();
    assert_eq!(b.sources().unwrap().len(), 2);
    assert_eq!(
        b.channels(Query {
            view: "favorites".into(),
            ..q()
        })
        .unwrap()
        .total,
        1
    );
    let _ = fs::remove_dir_all(dir);
}

#[test]
fn favorites_backup_survives_re_add() {
    let dir = temp_dir();
    let (lib, s) = test_library(&dir);
    let first = lib
        .channels(Query {
            sort: "az".into(),
            ..q()
        })
        .unwrap()
        .items[0]
        .id
        .clone();
    lib.set_favorite(&first, true).unwrap();
    let backup = lib.export_favorites().unwrap();
    let file = dir.join("backup.json");
    write_backup(&file, &backup).unwrap();

    // Delete and re-add the list: new source ID, so favourites are gone...
    let srv = fake_provider();
    lib.remove(&s.id).unwrap();
    block(lib.add(NewSource {
        url: format!("{srv}/big.m3u"),
        ..src(Kind::M3uUrl, "LIST")
    }))
    .unwrap();
    // ...until the backup brings them back, matched by names.
    assert_eq!(
        lib.import_favorites(read_backup(&file).unwrap()).unwrap(),
        1
    );
    let foreign: FavoritesBackup = serde_json::from_str(r#"{"format":"other"}"#).unwrap();
    assert!(lib.import_favorites(foreign).is_err());
    let _ = fs::remove_dir_all(dir);
}

#[test]
fn orphan_channel_files_are_removed() {
    let dir = temp_dir();
    test_library(&dir);
    let orphan = dir.join("channels").join("0123456789ab.json");
    fs::write(&orphan, "[]").unwrap();
    let old = SystemTime::now() - sync::ORPHAN_GRACE * 2;
    fs::File::options()
        .write(true)
        .open(&orphan)
        .unwrap()
        .set_modified(old)
        .unwrap();
    Library::open(dir.clone()).unwrap();
    assert!(!orphan.exists(), "orphan channel file survived");
    let _ = fs::remove_dir_all(dir);
}

/// Times the hot paths on a real library, with the same scenarios and method
/// as TestBenchRealLibrary in bench_test.go. Run with
/// `HERAKLEO_BENCH_DIR=<copy> cargo test --release bench_real_library -- --ignored --nocapture`.
#[test]
#[ignore]
fn bench_real_library() {
    let Ok(dir) = std::env::var("HERAKLEO_BENCH_DIR") else {
        return;
    };
    fn measure(name: &str, runs: usize, mut f: impl FnMut()) {
        f(); // warm-up
        let mut d: Vec<Duration> = (0..runs)
            .map(|_| {
                let start = std::time::Instant::now();
                f();
                start.elapsed()
            })
            .collect();
        d.sort();
        let ms = |x: Duration| x.as_micros() as f64 / 1000.0;
        println!(
            "BENCH rust {name:<14} median {:8.2} ms  p95 {:8.2} ms",
            ms(d[d.len() / 2]),
            ms(d[d.len() * 95 / 100])
        );
    }
    let q = |search: &str, group: &str, sort: &str| Query {
        source_id: String::new(),
        group: group.into(),
        search: search.into(),
        view: String::new(),
        sort: sort.into(),
        offset: 0,
        limit: 120,
    };

    let mut lib = None;
    measure("open", 5, || {
        lib = Some(Library::open(PathBuf::from(&dir)).unwrap())
    });
    let lib = lib.unwrap();
    let group = lib.groups("").unwrap()[0].name.clone();
    measure("page", 50, || drop(lib.channels(q("", "", "")).unwrap()));
    measure("page+json", 50, || {
        drop(serde_json::to_vec(&lib.channels(q("", "", "")).unwrap()).unwrap())
    });
    measure("search_common", 50, || {
        drop(lib.channels(q("es", "", "")).unwrap())
    });
    measure("search_rare", 50, || {
        drop(lib.channels(q("zzqx", "", "")).unwrap())
    });
    measure("group", 50, || {
        drop(lib.channels(q("", &group, "")).unwrap())
    });
    measure("sort_az", 50, || {
        drop(lib.channels(q("", "", "az")).unwrap())
    });
    measure("groups", 50, || drop(lib.groups("").unwrap()));
}
