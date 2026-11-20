package main

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/crstian19/herakleo/wails/internal/library"
	"github.com/crstian19/herakleo/wails/internal/update"
)

// fakeRelease serves the GitHub API and the downloads for one release.
// signed=false leaves SHA256SUMS.sig out.
func fakeRelease(t *testing.T, version string, files map[string][]byte, signed bool) (api, base string, pub ed25519.PublicKey) {
	t.Helper()
	pub, priv, _ := ed25519.GenerateKey(rand.Reader)
	var sums bytes.Buffer
	for name, b := range files {
		h := sha256.Sum256(b)
		sums.WriteString(hex.EncodeToString(h[:]) + "  " + name + "\n")
	}
	all := map[string][]byte{update.SumsName: sums.Bytes()}
	if signed {
		all[update.SigName] = ed25519.Sign(priv, sums.Bytes())
	}
	for k, v := range files {
		all[k] = v
	}
	assets := make([]map[string]string, 0, len(all))
	for name := range all {
		assets = append(assets, map[string]string{"name": name})
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/latest", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"tag_name": "v" + version, "body": "notes", "assets": assets})
	})
	for name, b := range all {
		mux.HandleFunc("/dl/v"+version+"/"+name, func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write(b) })
	}
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv.URL + "/latest", srv.URL + "/dl/", pub
}

func tarball(t *testing.T, version, body string) []byte {
	t.Helper()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	name := "herakleo-" + version + "-linux-x86_64/herakleo"
	_ = tw.WriteHeader(&tar.Header{Name: name, Mode: 0o755, Size: int64(len(body)), Typeflag: tar.TypeReg})
	_, _ = tw.Write([]byte(body))
	_ = tw.Close()
	_ = gz.Close()
	return buf.Bytes()
}

func updaterApp(t *testing.T, api, base string, key ed25519.PublicKey, in update.Install) *App {
	t.Helper()
	a := newApp(t)
	a.updates.apiURL, a.updates.base, a.updates.key = api, base, key
	a.updates.detect = func() update.Install { return in }
	return a
}

const next = "99.0.0"

func TestUpdateInstallsSelfOwnedInstall(t *testing.T) {
	quits := stubQuit(t)
	exe := filepath.Join(t.TempDir(), "herakleo")
	_ = os.WriteFile(exe, []byte("old"), 0o755) //nolint:gosec
	in := update.Install{Mode: update.ModeInstall, Method: update.MethodTarball, Target: exe}
	api, base, pub := fakeRelease(t, next, map[string][]byte{in.Asset(next): tarball(t, next, "new")}, true)
	a := updaterApp(t, api, base, pub, in)

	info, err := a.CheckUpdate(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !info.Available || info.Mode != "install" || info.Latest != next || info.Current != Version || info.Notes != "notes" {
		t.Fatalf("info %+v", info)
	}
	if err := a.InstallUpdate(context.Background()); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(exe); string(b) != "new" {
		t.Fatalf("exe = %q", b)
	}
	select {
	case <-quits:
	case <-time.After(5 * time.Second):
		t.Fatal("the app did not quit after the update")
	}
	if a.updates.restart == nil {
		t.Fatal("no restart queued")
	}
	// The release is used once: a second install has nothing to do.
	if err := a.InstallUpdate(context.Background()); err == nil || err.Error() != "update_none" {
		t.Fatalf("second install: %v", err)
	}
}

func TestUpdateCheckModes(t *testing.T) {
	exe := filepath.Join(t.TempDir(), "herakleo")
	self := update.Install{Mode: update.ModeInstall, Method: update.MethodTarball, Target: exe}
	dmg := update.Install{Mode: update.ModeInstall, Method: update.MethodDMG, Target: "/Applications/Herakleo.app"}
	scoop := update.Install{Mode: update.ModeCommand, Manager: "scoop", Command: "scoop update herakleo"}
	tests := []struct {
		name      string
		version   string
		in        update.Install
		files     map[string][]byte
		signed    bool
		available bool
		mode      string
	}{
		{"same version", Version, self, map[string][]byte{self.Asset(Version): []byte("x")}, true, false, "install"},
		{"older version", "0.0.1", self, map[string][]byte{self.Asset("0.0.1"): []byte("x")}, true, false, "install"},
		{"asset not uploaded yet", next, dmg, map[string][]byte{self.Asset(next): []byte("x")}, true, false, "install"},
		{"unsigned release is only a download", next, self, map[string][]byte{self.Asset(next): []byte("x")}, false, true, "download"},
		{"package manager", next, scoop, nil, true, true, "command"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			api, base, pub := fakeRelease(t, tt.version, tt.files, tt.signed)
			a := updaterApp(t, api, base, pub, tt.in)
			info, err := a.CheckUpdate(context.Background())
			if err != nil {
				t.Fatal(err)
			}
			if info.Available != tt.available || info.Mode != tt.mode {
				t.Fatalf("info %+v", info)
			}
			if tt.in.Mode == update.ModeCommand && info.Command != tt.in.Command {
				t.Fatalf("command %q", info.Command)
			}
			// Only an installable, available release can be installed.
			if err := a.InstallUpdate(context.Background()); err == nil || err.Error() != "update_none" {
				t.Fatalf("install: %v", err)
			}
		})
	}
}

// releaser-pleaser publishes the release before the packages: until
// SHA256SUMS is attached, no install mode offers it.
func TestUpdateWaitsForPackages(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"tag_name":"v` + next + `","assets":[]}`))
	}))
	t.Cleanup(srv.Close)
	for _, in := range []update.Install{
		{Mode: update.ModeCommand, Manager: "aur", Command: "yay -Syu"},
		{Mode: update.ModeDownload, Manager: "deb"},
	} {
		a := updaterApp(t, srv.URL, "", nil, in)
		info, err := a.CheckUpdate(context.Background())
		if err != nil || info.Available {
			t.Fatalf("%s: %+v %v", in.Mode, info, err)
		}
	}
}

func TestRestartRunsOnce(t *testing.T) {
	a := newApp(t)
	n := 0
	a.updates.restart = func() error { n++; return nil }
	_ = a.restartAfterUpdate()
	_ = a.restartAfterUpdate()
	if n != 1 {
		t.Fatalf("restart ran %d times", n)
	}
}

func TestUpdateCheckFailsWithCode(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))
	t.Cleanup(srv.Close)
	a := updaterApp(t, srv.URL, "", nil, update.Install{Mode: update.ModeDownload})
	if _, err := a.CheckUpdate(context.Background()); err == nil || !strings.HasPrefix(err.Error(), "update_check_failed: ") {
		t.Fatalf("err = %v", err)
	}
}

func TestUpdatesSetting(t *testing.T) {
	a := newApp(t)
	if a.GetSettings().Updates != library.UpdatesOn {
		t.Fatal("on by default")
	}
	bad := "sometimes"
	if _, err := a.UpdateSettings(library.SettingsPatch{Updates: &bad}); err == nil {
		t.Fatal("invalid value accepted")
	}
	off := library.UpdatesOff
	v, err := a.UpdateSettings(library.SettingsPatch{Updates: &off})
	if err != nil || v.Updates != library.UpdatesOff {
		t.Fatalf("%+v %v", v, err)
	}
}

func TestReleaseLinkIsAllowed(t *testing.T) {
	if u := projectLinks["release"]; !strings.HasPrefix(u, "https://github.com/crstian19/herakleo/releases") {
		t.Fatalf("release link %q", u)
	}
}
