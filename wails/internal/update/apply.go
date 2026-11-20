package update

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"time"
)

const (
	maxAsset = 256 << 20
	maxSums  = 64 << 10
	maxBin   = 256 << 20
)

// Updater downloads and applies a release for one Install.
type Updater struct {
	Client *http.Client
	// Base is DownloadBase unless a test serves the files.
	Base   string
	Key    []byte
	Target Install
}

// Restart starts the new version; call it after the app has quit, so the new
// process does not meet the old one's files, locks or D-Bus name.
type Restart func() error

// Apply downloads the release's asset for u.Target, checks it against the
// signed SHA256SUMS and installs it. The running app keeps working until it
// quits; the returned Restart then starts the new version.
func (u *Updater) Apply(ctx context.Context, rel Release) (Restart, error) {
	if u.Target.Mode != ModeInstall {
		return nil, errors.New("update_not_installable")
	}
	asset := u.Target.Asset(rel.Version)
	base := u.Base + "v" + rel.Version + "/"

	entries, err := u.signedSums(ctx, base)
	if err != nil {
		return nil, err
	}
	want, ok := entries[asset]
	if !ok {
		return nil, ErrUnverified
	}

	// Stage next to the target, so the final rename stays on one filesystem.
	dir := filepath.Dir(u.Target.Target)
	tmp, err := os.MkdirTemp(dir, ".herakleo-update-*")
	if err != nil {
		return nil, fmt.Errorf("update_failed: %w", err)
	}
	keep := false
	defer func() {
		if !keep {
			_ = os.RemoveAll(tmp)
		}
	}()
	file := filepath.Join(tmp, asset)
	if err := u.fetchFile(ctx, base+asset, file, want); err != nil {
		return nil, err
	}

	var restart Restart
	switch u.Target.Method {
	case MethodTarball:
		restart, err = applyArchive(file, tmp, u.Target.Target, extractTar,
			path.Join(fmt.Sprintf("herakleo-%s-linux-x86_64", rel.Version), "herakleo"))
	case MethodZip:
		restart, err = applyArchive(file, tmp, u.Target.Target, extractZip,
			path.Join(fmt.Sprintf("herakleo-%s-windows-x86_64", rel.Version), "herakleo.exe"))
	case MethodNSIS:
		keep = true // the installer runs from here after the app quits
		restart, err = applyNSIS(file, tmp, u.Target.Target)
	case MethodDMG:
		restart, err = applyDMG(ctx, file, tmp, u.Target.Target)
	default:
		err = errors.New("update_not_installable")
	}
	if err != nil {
		keep = false
		if errors.Is(err, ErrUnverified) {
			return nil, err
		}
		return nil, fmt.Errorf("update_failed: %w", err)
	}
	return restart, nil
}

// retryDelay separates the two tries of signedSums; tests shorten it.
var retryDelay = 5 * time.Second

// signedSums fetches and verifies SHA256SUMS. The release workflow uploads it
// and its .sig one after the other, so a mismatch is tried once more.
func (u *Updater) signedSums(ctx context.Context, base string) (map[string]string, error) {
	for try := 0; ; try++ {
		sums, err := u.fetchBytes(ctx, base+SumsName, maxSums)
		if err != nil {
			return nil, fmt.Errorf("update_download_failed: %w", err)
		}
		sig, err := u.fetchBytes(ctx, base+SigName, 1024)
		if err != nil {
			return nil, fmt.Errorf("update_download_failed: %w", err)
		}
		entries, err := VerifySums(sums, sig, u.Key)
		if err == nil || try == 1 {
			return entries, err
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(retryDelay):
		}
	}
}

func (u *Updater) get(ctx context.Context, url string) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "herakleo-updater")
	resp, err := u.Client.Do(req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		_ = resp.Body.Close()
		return nil, fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	return resp, nil
}

func (u *Updater) fetchBytes(ctx context.Context, url string, limit int64) ([]byte, error) {
	resp, err := u.get(ctx, url)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	b, err := io.ReadAll(io.LimitReader(resp.Body, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(b)) > limit {
		return nil, errors.New("file too large")
	}
	return b, nil
}

// fetchFile saves url to dst, failing with ErrUnverified unless its SHA-256
// is want.
func (u *Updater) fetchFile(ctx context.Context, url, dst, want string) error {
	resp, err := u.get(ctx, url)
	if err != nil {
		return fmt.Errorf("update_download_failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	f, err := os.OpenFile(dst, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600) //nolint:gosec // dst is our own staging path
	if err != nil {
		return fmt.Errorf("update_failed: %w", err)
	}
	h := sha256.New()
	n, err := io.Copy(io.MultiWriter(f, h), io.LimitReader(resp.Body, maxAsset+1))
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		return fmt.Errorf("update_download_failed: %w", err)
	}
	if n > maxAsset || hex.EncodeToString(h.Sum(nil)) != want {
		return ErrUnverified
	}
	return nil
}

type extractor func(archive, inner string, w io.Writer) error

// applyArchive pulls the binary out of a verified archive and swaps it in.
// The old binary is renamed to <exe>.old first (Windows cannot overwrite a
// running .exe but can rename it); Cleanup removes it on the next start.
func applyArchive(archive, tmp, exe string, extract extractor, inner string) (Restart, error) {
	next := filepath.Join(tmp, "next")
	f, err := os.OpenFile(next, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o755) //nolint:gosec // an executable
	if err != nil {
		return nil, err
	}
	err = extract(archive, inner, f)
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		return nil, err
	}
	if err := swap(next, exe); err != nil {
		return nil, err
	}
	return func() error { return startDetached(exe) }, nil
}

// swap puts next in place of exe, keeping exe as exe.old, and puts exe back
// if the second rename fails.
func swap(next, exe string) error {
	old := exe + ".old"
	_ = os.Remove(old)
	if err := os.Rename(exe, old); err != nil {
		return err
	}
	if err := os.Rename(next, exe); err != nil {
		_ = os.Rename(old, exe)
		return err
	}
	return nil
}

// Cleanup removes what a previous update left behind: exe.old and the
// staging folders next to it (the Windows installer runs from one). Call it
// at start, with the Install's Target.
func Cleanup(target string) {
	if target == "" {
		return
	}
	_ = os.RemoveAll(target + ".old")
	stale, _ := filepath.Glob(filepath.Join(filepath.Dir(target), ".herakleo-update-*"))
	for _, d := range stale {
		// Another copy of the app may be updating right now.
		if fi, err := os.Stat(d); err == nil && time.Since(fi.ModTime()) > time.Hour {
			_ = os.RemoveAll(d)
		}
	}
}

func extractTar(archive, inner string, w io.Writer) error {
	f, err := os.Open(archive) //nolint:gosec // our own staging file
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()
	gz, err := gzip.NewReader(f)
	if err != nil {
		return err
	}
	tr := tar.NewReader(gz)
	for {
		h, err := tr.Next()
		if errors.Is(err, io.EOF) {
			return fmt.Errorf("%s not in the archive", inner)
		}
		if err != nil {
			return err
		}
		if path.Clean(h.Name) == inner && h.Typeflag == tar.TypeReg {
			_, err := io.Copy(w, io.LimitReader(tr, maxBin))
			return err
		}
	}
}

func extractZip(archive, inner string, w io.Writer) error {
	zr, err := zip.OpenReader(archive)
	if err != nil {
		return err
	}
	defer func() { _ = zr.Close() }()
	for _, zf := range zr.File {
		if path.Clean(zf.Name) != inner || !zf.Mode().IsRegular() {
			continue
		}
		r, err := zf.Open()
		if err != nil {
			return err
		}
		defer func() { _ = r.Close() }()
		_, err = io.Copy(w, io.LimitReader(r, maxBin))
		return err
	}
	return fmt.Errorf("%s not in the archive", inner)
}

// applyDMG copies Herakleo.app out of the mounted image next to the current
// one and swaps the two folders. Files fetched by the app carry no quarantine
// flag, so Gatekeeper does not ask again.
func applyDMG(ctx context.Context, dmg, tmp, app string) (Restart, error) {
	mnt := filepath.Join(tmp, "mnt")
	if err := exec.CommandContext(ctx, "hdiutil", "attach", "-nobrowse", "-readonly", "-noautoopen",
		"-mountpoint", mnt, dmg).Run(); err != nil {
		return nil, fmt.Errorf("hdiutil attach: %w", err)
	}
	defer func() {
		dctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		_ = exec.CommandContext(dctx, "hdiutil", "detach", "-force", mnt).Run()
	}()

	next := filepath.Join(tmp, "Herakleo.app")
	if err := exec.CommandContext(ctx, "ditto", filepath.Join(mnt, "Herakleo.app"), next).Run(); err != nil {
		return nil, fmt.Errorf("ditto: %w", err)
	}
	// Herakleo.app.old stays until Cleanup on the next start, in case the new one fails.
	if err := swap(next, app); err != nil {
		return nil, err
	}
	return func() error { return exec.CommandContext(context.Background(), "open", "-n", app).Start() }, nil
}
