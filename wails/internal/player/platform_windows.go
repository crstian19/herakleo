package player

import (
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
)

// Installers rarely add players to PATH on Windows; Scoop and Chocolatey do,
// and are covered by the PATH lookup.
func findPlayer(id, bin string) (string, error) {
	var paths []string
	for _, env := range []string{"ProgramFiles", "ProgramFiles(x86)", "LOCALAPPDATA"} {
		base := os.Getenv(env)
		if base == "" {
			continue
		}
		if env == "LOCALAPPDATA" {
			base = filepath.Join(base, "Programs")
		}
		switch id {
		case "mpv":
			paths = append(paths, filepath.Join(base, "mpv", "mpv.exe"))
		case "vlc":
			paths = append(paths, filepath.Join(base, "VideoLAN", "VLC", "vlc.exe"))
		}
	}
	return lookPathOr(bin, paths)
}

// createNoWindow keeps console launchers (Scoop's shims are console
// programs) from flashing a terminal; GUI players are unaffected.
const createNoWindow = 0x08000000

func playerCommand(bin string, args []string) *exec.Cmd {
	cmd := exec.Command(bin, args...)
	cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: createNoWindow}
	return cmd
}

func stopProcess(p *os.Process) { _ = p.Kill() }

func runtimeDir() string { return "" }
