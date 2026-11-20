package update

import "testing"

func TestDetect(t *testing.T) {
	type machine struct {
		goos, arch, exe string
		flatpak         bool
		files           []string // paths that exist (also matched by Glob as prefixes)
		readOnly        bool
	}
	tests := []struct {
		name    string
		m       machine
		mode    Mode
		method  Method
		manager string
		command string
	}{
		{"linux tarball in home", machine{goos: "linux", arch: "amd64", exe: "/home/u/apps/herakleo/herakleo"}, ModeInstall, MethodTarball, "", ""},
		{"linux tarball read-only", machine{goos: "linux", arch: "amd64", exe: "/srv/herakleo", readOnly: true}, ModeDownload, "", "", ""},
		{"linux arm64 has no build", machine{goos: "linux", arch: "arm64", exe: "/home/u/herakleo"}, ModeDownload, "", "", ""},
		{"flatpak", machine{goos: "linux", arch: "amd64", exe: "/app/bin/herakleo", flatpak: true}, ModeDownload, "", "flatpak", ""},
		{"aur source", machine{goos: "linux", arch: "amd64", exe: "/usr/bin/herakleo", files: []string{"/var/lib/pacman/local/herakleo-0.4.4-1"}}, ModeCommand, "", "aur", "yay -S herakleo"},
		{"aur bin", machine{goos: "linux", arch: "amd64", exe: "/usr/bin/herakleo", files: []string{"/var/lib/pacman/local/herakleo-bin-0.4.4-1"}}, ModeCommand, "", "aur", "yay -S herakleo-bin"},
		{"windows exe without a folder", machine{goos: "windows", arch: "amd64", exe: "herakleo.exe"}, ModeDownload, "", "", ""},
		{"deb", machine{goos: "linux", arch: "amd64", exe: "/usr/bin/herakleo", files: []string{"/var/lib/dpkg/info/herakleo.list"}}, ModeDownload, "", "deb", ""},
		{"rpm", machine{goos: "linux", arch: "amd64", exe: "/usr/bin/herakleo", files: []string{"/usr/lib/sysimage/rpm"}}, ModeDownload, "", "rpm", ""},
		{"rpm, old database path", machine{goos: "linux", arch: "amd64", exe: "/usr/bin/herakleo", files: []string{"/var/lib/rpm"}}, ModeDownload, "", "rpm", ""},
		{"system dir, no known package", machine{goos: "linux", arch: "amd64", exe: "/usr/bin/herakleo"}, ModeDownload, "", "", ""},
		{"copied to /usr/local by hand", machine{goos: "linux", arch: "amd64", exe: "/usr/local/bin/herakleo", files: []string{"/usr/lib/sysimage/rpm"}}, ModeDownload, "", "", ""},
		{"windows installer", machine{goos: "windows", arch: "amd64", exe: `C:\Users\u\AppData\Local\Programs\Herakleo\herakleo.exe`, files: []string{`C:\Users\u\AppData\Local\Programs\Herakleo\uninstall.exe`}}, ModeInstall, MethodNSIS, "", ""},
		{"windows zip", machine{goos: "windows", arch: "amd64", exe: `D:\Tools\herakleo\herakleo.exe`}, ModeInstall, MethodZip, "", ""},
		{"scoop", machine{goos: "windows", arch: "amd64", exe: `C:\Users\u\scoop\apps\herakleo\0.4.4\herakleo.exe`}, ModeCommand, "", "scoop", ""},
		{"scoop any case", machine{goos: "windows", arch: "amd64", exe: `C:\Users\u\Scoop\Apps\Herakleo\current\herakleo.exe`}, ModeCommand, "", "scoop", ""},
		{"windows read-only", machine{goos: "windows", arch: "amd64", exe: `C:\Program Files\Herakleo\herakleo.exe`, readOnly: true}, ModeDownload, "", "", ""},
		{"mac dmg", machine{goos: "darwin", arch: "arm64", exe: "/Applications/Herakleo.app/Contents/MacOS/herakleo"}, ModeInstall, MethodDMG, "", ""},
		{"mac homebrew", machine{goos: "darwin", arch: "arm64", exe: "/Applications/Herakleo.app/Contents/MacOS/herakleo", files: []string{"/opt/homebrew/Caskroom/herakleo"}}, ModeCommand, "", "brew", ""},
		{"mac homebrew intel", machine{goos: "darwin", arch: "amd64", exe: "/Applications/Herakleo.app/Contents/MacOS/herakleo", files: []string{"/usr/local/Caskroom/herakleo"}}, ModeCommand, "", "brew", ""},
		{"mac bare binary", machine{goos: "darwin", arch: "arm64", exe: "/Users/u/go/bin/herakleo"}, ModeDownload, "", "", ""},
		{"mac read-only Applications", machine{goos: "darwin", arch: "arm64", exe: "/Applications/Herakleo.app/Contents/MacOS/herakleo", readOnly: true}, ModeDownload, "", "", ""},
		{"unknown exe", machine{goos: "linux", arch: "amd64"}, ModeDownload, "", "", ""},
		{"other os", machine{goos: "freebsd", arch: "amd64", exe: "/usr/local/bin/herakleo"}, ModeDownload, "", "", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := tt.m
			has := func(p string) bool {
				for _, f := range m.files {
					if f == p {
						return true
					}
				}
				return false
			}
			env := Env{
				GOOS: m.goos, GOARCH: m.arch, Exe: m.exe, Flatpak: m.flatpak,
				Exists:   has,
				Writable: func(string) bool { return !m.readOnly },
				Glob: func(pattern string) []string {
					var out []string
					for _, f := range m.files {
						if ok, _ := matchGlob(pattern, f); ok {
							out = append(out, f)
						}
					}
					return out
				},
			}
			got := Detect(env)
			if got.Mode != tt.mode || got.Method != tt.method || got.Manager != tt.manager {
				t.Fatalf("Detect = %+v, want mode %s method %s manager %s", got, tt.mode, tt.method, tt.manager)
			}
			if got.Mode == ModeCommand && got.Command == "" {
				t.Fatal("command mode without a command")
			}
			if tt.command != "" && got.Command != tt.command {
				t.Fatalf("command %q, want %q", got.Command, tt.command)
			}
			if got.Mode == ModeInstall && (got.Target == "" || got.Asset("1.2.3") == "") {
				t.Fatalf("install mode without target/asset: %+v", got)
			}
		})
	}
}
