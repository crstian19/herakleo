package update

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"
)

func TestMain(m *testing.M) {
	retryDelay = time.Millisecond // signature mismatches retry once
	os.Exit(m.Run())
}

type tarEntry struct {
	name string
	body string
	typ  byte
}

func makeTar(t *testing.T, entries ...tarEntry) []byte {
	t.Helper()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	for _, e := range entries {
		h := &tar.Header{Name: e.name, Mode: 0o755, Size: int64(len(e.body)), Typeflag: e.typ}
		if e.typ == tar.TypeSymlink {
			h.Size, h.Linkname = 0, "/etc/passwd"
		}
		if err := tw.WriteHeader(h); err != nil {
			t.Fatal(err)
		}
		if e.typ == tar.TypeReg {
			_, _ = tw.Write([]byte(e.body))
		}
	}
	_ = tw.Close()
	_ = gz.Close()
	return buf.Bytes()
}

func makeZip(t *testing.T, name, body string) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	w, _ := zw.Create(name)
	_, _ = w.Write([]byte(body))
	_ = zw.Close()
	return buf.Bytes()
}

// release serves a fake v<version> release: files by name, SHA256SUMS over
// sumFiles (defaults to files) and its signature by priv.
func release(t *testing.T, priv ed25519.PrivateKey, version string, files map[string][]byte, sumFiles map[string][]byte) string {
	t.Helper()
	if sumFiles == nil {
		sumFiles = files
	}
	var sums bytes.Buffer
	for name, b := range sumFiles {
		h := sha256.Sum256(b)
		sums.WriteString(hex.EncodeToString(h[:]) + "  " + name + "\n")
	}
	all := map[string][]byte{SumsName: sums.Bytes(), SigName: ed25519.Sign(priv, sums.Bytes())}
	for k, v := range files {
		all[k] = v
	}
	mux := http.NewServeMux()
	for name, b := range all {
		mux.HandleFunc("/v"+version+"/"+name, func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write(b) })
	}
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv.URL + "/"
}

func installedExe(t *testing.T, name string) string {
	t.Helper()
	exe := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(exe, []byte("old"), 0o755); err != nil { //nolint:gosec
		t.Fatal(err)
	}
	return exe
}

func TestApplyArchives(t *testing.T) {
	pub, priv := newKey(t)
	const v = "0.5.0"
	tests := []struct {
		name   string
		method Method
		exe    string
		asset  []byte
	}{
		{"tarball", MethodTarball, "herakleo", makeTar(t,
			tarEntry{"herakleo-0.5.0-linux-x86_64/LICENSE", "license", tar.TypeReg},
			tarEntry{"herakleo-0.5.0-linux-x86_64/herakleo", "new", tar.TypeReg})},
		{"zip", MethodZip, "herakleo.exe", makeZip(t, "herakleo-0.5.0-windows-x86_64/herakleo.exe", "new")},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			exe := installedExe(t, tt.exe)
			in := Install{Mode: ModeInstall, Method: tt.method, Target: exe}
			base := release(t, priv, v, map[string][]byte{in.Asset(v): tt.asset}, nil)
			u := &Updater{Client: http.DefaultClient, Base: base, Key: pub, Target: in}

			restart, err := u.Apply(context.Background(), Release{Version: v})
			if err != nil {
				t.Fatal(err)
			}
			if restart == nil {
				t.Fatal("no restart")
			}
			if b, _ := os.ReadFile(exe); string(b) != "new" {
				t.Fatalf("exe = %q", b)
			}
			fi, _ := os.Stat(exe)
			if fi.Mode()&0o100 == 0 {
				t.Fatalf("new exe is not executable: %v", fi.Mode())
			}
			if b, _ := os.ReadFile(exe + ".old"); string(b) != "old" {
				t.Fatalf("old = %q", b)
			}
			if stale, _ := filepath.Glob(filepath.Join(filepath.Dir(exe), ".herakleo-update-*")); len(stale) != 0 {
				t.Fatalf("staging left behind: %v", stale)
			}
			Cleanup(exe)
			if _, err := os.Stat(exe + ".old"); !os.IsNotExist(err) {
				t.Fatal("Cleanup kept exe.old")
			}
		})
	}
}

// Anything that fails verification or extraction leaves the install as it was.
func TestApplyRefuses(t *testing.T) {
	pub, priv := newKey(t)
	otherPub, _ := newKey(t)
	const v = "0.5.0"
	asset := "herakleo-0.5.0-linux-x86_64.tar.gz"
	good := makeTar(t, tarEntry{"herakleo-0.5.0-linux-x86_64/herakleo", "new", tar.TypeReg})
	tests := []struct {
		name    string
		key     ed25519.PublicKey
		files   map[string][]byte
		sums    map[string][]byte
		wantErr error
	}{
		{"wrong key", otherPub, map[string][]byte{asset: good}, nil, ErrUnverified},
		{"asset swapped after signing", pub, map[string][]byte{asset: good}, map[string][]byte{asset: []byte("other")}, ErrUnverified},
		{"asset not in sums", pub, map[string][]byte{asset: good}, map[string][]byte{"else": good}, ErrUnverified},
		{"traversal entry only", pub, map[string][]byte{asset: makeTar(t,
			tarEntry{"../herakleo-0.5.0-linux-x86_64/herakleo", "evil", tar.TypeReg},
			tarEntry{"herakleo", "evil", tar.TypeReg})}, nil, nil},
		{"symlink instead of binary", pub, map[string][]byte{asset: makeTar(t,
			tarEntry{"herakleo-0.5.0-linux-x86_64/herakleo", "", tar.TypeSymlink})}, nil, nil},
		{"not gzip", pub, map[string][]byte{asset: []byte("plain")}, nil, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			exe := installedExe(t, "herakleo")
			u := &Updater{
				Client: http.DefaultClient, Base: release(t, priv, v, tt.files, tt.sums), Key: tt.key,
				Target: Install{Mode: ModeInstall, Method: MethodTarball, Target: exe},
			}
			_, err := u.Apply(context.Background(), Release{Version: v})
			if err == nil {
				t.Fatal("applied")
			}
			if tt.wantErr != nil && !errors.Is(err, tt.wantErr) {
				t.Fatalf("err = %v, want %v", err, tt.wantErr)
			}
			if b, _ := os.ReadFile(exe); string(b) != "old" {
				t.Fatalf("exe changed to %q", b)
			}
			entries, _ := os.ReadDir(filepath.Dir(exe))
			if len(entries) != 1 {
				t.Fatalf("left files behind: %v", entries)
			}
		})
	}
}

// SHA256SUMS and its .sig are uploaded one after the other: a mismatch is
// retried once before giving up.
func TestApplyRetriesSignatureOnce(t *testing.T) {
	pub, priv := newKey(t)
	const v = "0.5.0"
	asset := makeTar(t, tarEntry{"herakleo-0.5.0-linux-x86_64/herakleo", "new", tar.TypeReg})
	name := "herakleo-0.5.0-linux-x86_64.tar.gz"
	h := sha256.Sum256(asset)
	sums := []byte(hex.EncodeToString(h[:]) + "  " + name + "\n")
	good := ed25519.Sign(priv, sums)
	for _, tt := range []struct {
		name     string
		badTries int
		ok       bool
	}{{"stale signature once", 1, true}, {"always wrong", 99, false}} {
		t.Run(tt.name, func(t *testing.T) {
			var sigHits atomic.Int32
			mux := http.NewServeMux()
			mux.HandleFunc("/v0.5.0/"+SumsName, func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write(sums) })
			mux.HandleFunc("/v0.5.0/"+SigName, func(w http.ResponseWriter, _ *http.Request) {
				if int(sigHits.Add(1)) <= tt.badTries {
					_, _ = w.Write(make([]byte, ed25519.SignatureSize))
					return
				}
				_, _ = w.Write(good)
			})
			mux.HandleFunc("/v0.5.0/"+name, func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write(asset) })
			srv := httptest.NewServer(mux)
			t.Cleanup(srv.Close)
			exe := installedExe(t, "herakleo")
			u := &Updater{
				Client: http.DefaultClient, Base: srv.URL + "/", Key: pub,
				Target: Install{Mode: ModeInstall, Method: MethodTarball, Target: exe},
			}
			_, err := u.Apply(context.Background(), Release{Version: v})
			if tt.ok != (err == nil) {
				t.Fatalf("err = %v", err)
			}
			if !tt.ok && !errors.Is(err, ErrUnverified) {
				t.Fatalf("err = %v, want ErrUnverified", err)
			}
			if got := sigHits.Load(); got != 2 {
				t.Fatalf("signature fetched %d times, want 2", got)
			}
		})
	}
}

func TestApplyNeedsInstallMode(t *testing.T) {
	u := &Updater{Target: Install{Mode: ModeCommand}}
	if _, err := u.Apply(context.Background(), Release{Version: "0.5.0"}); err == nil {
		t.Fatal("applied a command-mode install")
	}
}
