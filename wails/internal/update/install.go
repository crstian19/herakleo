package update

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Mode is what the app can do about a new version.
type Mode string

const (
	// ModeInstall means the app downloads the new version and replaces itself.
	ModeInstall Mode = "install"
	// ModeCommand means a package manager owns the install; the user runs Command.
	ModeCommand Mode = "command"
	// ModeDownload means the user downloads the new package (deb, rpm, Flatpak
	// bundle, read-only folders, unsupported architectures).
	ModeDownload Mode = "download"
)

// Method is how ModeInstall swaps the files.
type Method string

// The ModeInstall methods, one per self-owned package.
const (
	MethodTarball Method = "tarball" // Linux: replace the binary from the .tar.gz
	MethodZip     Method = "zip"     // Windows portable: replace the .exe from the .zip
	MethodNSIS    Method = "nsis"    // Windows installer: run the new setup.exe silently
	MethodDMG     Method = "dmg"     // macOS: replace Herakleo.app from the .dmg
)

// Install describes how this copy of Herakleo was installed.
type Install struct {
	Mode   Mode
	Method Method
	// Manager names what owns the install: aur, deb, rpm, flatpak, scoop, brew.
	Manager string
	// Command updates a ModeCommand install.
	Command string
	// Target is the executable (tarball, zip, nsis) or the .app (dmg) to replace.
	Target string
}

// Env is what Detect looks at, so tests can describe any machine.
type Env struct {
	GOOS, GOARCH string
	// Exe is the running executable with symlinks resolved.
	Exe      string
	Flatpak  bool
	Exists   func(path string) bool
	Writable func(dir string) bool
	Glob     func(pattern string) []string
}

// SystemEnv describes the running process.
func SystemEnv(goos, goarch string) Env {
	exe, err := os.Executable()
	if err == nil {
		if r, err := filepath.EvalSymlinks(exe); err == nil {
			exe = r
		}
	}
	_, ferr := os.Stat("/.flatpak-info")
	return Env{
		GOOS: goos, GOARCH: goarch, Exe: exe, Flatpak: goos == "linux" && ferr == nil,
		Exists: func(p string) bool {
			_, err := os.Stat(p)
			return err == nil
		},
		Writable: writable,
		Glob: func(p string) []string {
			m, _ := filepath.Glob(p)
			return m
		},
	}
}

func writable(dir string) bool {
	f, err := os.CreateTemp(dir, ".herakleo-write-test-*")
	if err != nil {
		return false
	}
	name := f.Name()
	_ = f.Close()
	_ = os.Remove(name)
	return true
}

// Detect works out who owns this install. When in doubt it answers
// ModeDownload: telling the user where the package is never breaks anything.
func Detect(e Env) Install {
	download := func(manager string) Install { return Install{Mode: ModeDownload, Manager: manager} }
	if e.Exe == "" {
		return download("")
	}
	switch e.GOOS {
	case "linux":
		return detectLinux(e, download)
	case "windows":
		return detectWindows(e, download)
	case "darwin":
		return detectDarwin(e, download)
	}
	return download("")
}

func detectLinux(e Env, download func(string) Install) Install {
	if e.Flatpak {
		return download("flatpak")
	}
	if strings.HasPrefix(e.Exe, "/usr/local/") {
		return download("") // copied there by hand
	}
	if strings.HasPrefix(e.Exe, "/usr/") {
		// Only this package, not a whole system upgrade.
		switch {
		case len(e.Glob("/var/lib/pacman/local/herakleo-bin-*")) > 0:
			return Install{Mode: ModeCommand, Manager: "aur", Command: "yay -S herakleo-bin"}
		case len(e.Glob("/var/lib/pacman/local/herakleo-[0-9]*")) > 0:
			return Install{Mode: ModeCommand, Manager: "aur", Command: "yay -S herakleo"}
		case e.Exists("/var/lib/dpkg/info/herakleo.list"):
			return download("deb")
		case e.Exists("/usr/lib/sysimage/rpm") || e.Exists("/var/lib/rpm"):
			return download("rpm")
		default:
			return download("")
		}
	}
	// Only x86_64 is built.
	if e.GOARCH != "amd64" || !e.Writable(filepath.Dir(e.Exe)) {
		return download("")
	}
	return Install{Mode: ModeInstall, Method: MethodTarball, Target: e.Exe}
}

func detectWindows(e Env, download func(string) Install) Install {
	if strings.Contains(strings.ToLower(e.Exe), `\scoop\apps\herakleo\`) {
		return Install{Mode: ModeCommand, Manager: "scoop", Command: "scoop update herakleo"}
	}
	// Windows paths are split by hand, so tests on other systems see them too.
	sep := strings.LastIndexAny(e.Exe, `\/`)
	if sep <= 0 {
		return download("")
	}
	dir := e.Exe[:sep]
	if e.GOARCH != "amd64" || !e.Writable(dir) {
		return download("")
	}
	if e.Exists(dir + `\uninstall.exe`) {
		return Install{Mode: ModeInstall, Method: MethodNSIS, Target: e.Exe}
	}
	return Install{Mode: ModeInstall, Method: MethodZip, Target: e.Exe}
}

func detectDarwin(e Env, download func(string) Install) Install {
	app, ok := strings.CutSuffix(e.Exe, "/Contents/MacOS/herakleo")
	if !ok || !strings.HasSuffix(app, ".app") {
		return download("")
	}
	if e.Exists("/opt/homebrew/Caskroom/herakleo") || e.Exists("/usr/local/Caskroom/herakleo") {
		return Install{Mode: ModeCommand, Manager: "brew", Command: "brew upgrade --cask herakleo"}
	}
	if !e.Writable(filepath.Dir(app)) {
		return download("")
	}
	return Install{Mode: ModeInstall, Method: MethodDMG, Target: app}
}

// Asset is the release file a ModeInstall install needs for version v.
func (i Install) Asset(v string) string {
	switch i.Method {
	case MethodTarball:
		return fmt.Sprintf("herakleo-%s-linux-x86_64.tar.gz", v)
	case MethodZip:
		return fmt.Sprintf("herakleo-%s-windows-x86_64.zip", v)
	case MethodNSIS:
		return fmt.Sprintf("herakleo-%s-windows-x86_64-setup.exe", v)
	case MethodDMG:
		return fmt.Sprintf("herakleo-%s-macos-universal.dmg", v)
	}
	return ""
}
