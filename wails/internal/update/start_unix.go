//go:build !windows

package update

import (
	"context"
	"errors"
	"os/exec"
	"syscall"
)

// startDetached starts exe in its own session, so it outlives this process.
func startDetached(exe string) error {
	// The new process must outlive this one: no cancelling context.
	cmd := exec.CommandContext(context.Background(), exe)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	return cmd.Start()
}

func applyNSIS(string, string, string) (Restart, error) {
	return nil, errors.New("the installer only runs on Windows")
}
