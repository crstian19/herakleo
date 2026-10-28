package player

import (
	"os"
	"os/exec"
	"path/filepath"
)

// Apps opened from Finder get a PATH of /usr/bin:/bin:/usr/sbin:/sbin, so
// Homebrew, MacPorts and app bundles are checked by hand.
func findPlayer(id, bin string) (string, error) {
	home, _ := os.UserHomeDir()
	var paths []string
	for _, dir := range []string{"/opt/homebrew/bin", "/usr/local/bin", "/opt/local/bin"} {
		paths = append(paths, filepath.Join(dir, bin))
	}
	for _, apps := range []string{"/Applications", filepath.Join(home, "Applications")} {
		switch id {
		case "mpv":
			paths = append(paths, filepath.Join(apps, "mpv.app/Contents/MacOS/mpv"))
		case "vlc":
			paths = append(paths, filepath.Join(apps, "VLC.app/Contents/MacOS/VLC"))
		case "iina":
			paths = append(paths, filepath.Join(apps, "IINA.app/Contents/MacOS/iina-cli"))
		}
	}
	return lookPathOr(bin, paths)
}

func playerCommand(bin string, args []string) *exec.Cmd { return exec.Command(bin, args...) }

func stopProcess(p *os.Process) { _ = p.Kill() }

func runtimeDir() string { return "" }
