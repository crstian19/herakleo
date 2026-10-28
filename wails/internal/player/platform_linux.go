package player

import (
	"os"
	"os/exec"
	"strings"
	"syscall"
)

// Inside a Flatpak the players live on the host, outside the sandbox, so
// they are found and started through flatpak-spawn. That needs the
// org.freedesktop.Flatpak talk permission in the manifest.
var inFlatpak = func() bool {
	_, err := os.Stat("/.flatpak-info")
	return err == nil
}()

func findPlayer(_, bin string) (string, error) {
	if !inFlatpak {
		return exec.LookPath(bin)
	}
	// bin is a fixed name from the allowlist, passed as $1, never spliced
	// into the script.
	out, err := exec.Command("flatpak-spawn", "--host", "sh", "-c", `command -v "$1"`, "sh", bin).Output()
	if p := strings.TrimSpace(string(out)); err == nil && strings.HasPrefix(p, "/") {
		return p, nil
	}
	return "", exec.ErrNotFound
}

func playerCommand(bin string, args []string) *exec.Cmd {
	if inFlatpak {
		return exec.Command("flatpak-spawn", append([]string{"--host", bin}, args...)...)
	}
	return exec.Command(bin, args...)
}

// flatpak-spawn forwards SIGTERM to the host player; a SIGKILL would only
// kill flatpak-spawn and leave the player running.
func stopProcess(p *os.Process) {
	if inFlatpak {
		_ = p.Signal(syscall.SIGTERM)
		return
	}
	_ = p.Kill()
}

// The sandbox has its own XDG_RUNTIME_DIR; only this subdirectory is the
// same path on the host, where the player reads the playlist.
func runtimeDir() string {
	dir := os.Getenv("XDG_RUNTIME_DIR")
	if id := os.Getenv("FLATPAK_ID"); inFlatpak && id != "" && dir != "" {
		return dir + "/app/" + id
	}
	return dir
}
