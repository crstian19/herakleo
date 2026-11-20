//go:build windows

package update

import (
	"context"
	"fmt"
	"os/exec"
	"path/filepath"
	"syscall"
)

const (
	createNoWindow  = 0x08000000
	detachedProcess = 0x00000008
	newProcessGroup = 0x00000200
)

func startDetached(exe string) error {
	// The new process must outlive this one: no cancelling context.
	cmd := exec.CommandContext(context.Background(), exe)
	cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: detachedProcess | newProcessGroup}
	return cmd.Start()
}

// applyNSIS returns a Restart that runs the verified installer silently into
// the app's folder. Nothing is touched before then: with /relaunch the
// installer waits until the running herakleo.exe can be overwritten, installs,
// and starts the app (project.nsi). If it fails, the old install is intact.
// The installer is per-user, so no UAC prompt appears.
func applyNSIS(setup, _, exe string) (Restart, error) {
	// No shell: NSIS reads its own command line. /D= must be last and unquoted.
	line := fmt.Sprintf(`%s /S /relaunch /D=%s`, syscall.EscapeArg(setup), filepath.Dir(exe))
	return func() error {
		// The line holds only our own paths (the install folder and the
		// verified installer), never anything from the network or the UI.
		cmd := exec.CommandContext(context.Background(), setup) //nolint:gosec
		cmd.SysProcAttr = &syscall.SysProcAttr{
			CmdLine:       line,
			CreationFlags: createNoWindow | detachedProcess | newProcessGroup,
		}
		return cmd.Start()
	}, nil
}
