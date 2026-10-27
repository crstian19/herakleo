package library

import (
	"context"
	"strconv"
	"testing"
)

func TestLogoColorFailuresBounded(t *testing.T) {
	l, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	l.colors = map[string]string{"http://ok/logo.png": "#dc1e28"}
	for i := range maxColorCache {
		l.colors["http://bad/"+strconv.Itoa(i)] = ""
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // no downloads: the new URL just fails
	l.LogoColors(ctx, []string{"http://new/logo.png"})

	if _, ok := l.colors["http://bad/0"]; ok || len(l.colors) != 2 {
		t.Fatalf("old failures not dropped: %d cached", len(l.colors))
	}
	if l.colors["http://ok/logo.png"] != "#dc1e28" {
		t.Fatal("a cached colour was dropped")
	}
}
