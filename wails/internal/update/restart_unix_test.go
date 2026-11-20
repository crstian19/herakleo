//go:build !windows

package update

import (
	"archive/tar"
	"context"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// The Restart from a tarball update starts the new binary, detached.
func TestTarballRestartStartsNewBinary(t *testing.T) {
	pub, priv := newKey(t)
	const v = "0.5.0"
	marker := filepath.Join(t.TempDir(), "started")
	script := "#!/bin/sh\ntouch '" + marker + "'\n"
	exe := installedExe(t, "herakleo")
	in := Install{Mode: ModeInstall, Method: MethodTarball, Target: exe}
	asset := makeTar(t, tarEntry{"herakleo-0.5.0-linux-x86_64/herakleo", script, tar.TypeReg})
	u := &Updater{Client: http.DefaultClient, Base: release(t, priv, v, map[string][]byte{in.Asset(v): asset}, nil), Key: pub, Target: in}

	restart, err := u.Apply(context.Background(), Release{Version: v})
	if err != nil {
		t.Fatal(err)
	}
	if err := restart(); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(marker); err == nil {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("the new binary did not run")
}
