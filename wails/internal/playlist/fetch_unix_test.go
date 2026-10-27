//go:build unix

package playlist

import (
	"path/filepath"
	"syscall"
	"testing"
	"time"
)

func TestM3UFromFileRejectsFIFO(t *testing.T) {
	path := filepath.Join(t.TempDir(), "list.m3u")
	if err := syscall.Mkfifo(path, 0o600); err != nil {
		t.Skip("mkfifo:", err)
	}
	done := make(chan error, 1)
	go func() {
		_, err := M3UFromFile(path)
		done <- err
	}()
	select {
	case err := <-done:
		if err == nil || err.Error() != "not_regular_file" {
			t.Fatalf("got %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("M3UFromFile blocked on a FIFO")
	}
}
