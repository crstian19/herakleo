package main

import (
	"context"
	"path/filepath"
	"slices"
	"testing"

	"github.com/crstian19/herakleo/wails/internal/library"
)

func TestSourceCallsRefuseUnknownIDs(t *testing.T) {
	a := newApp(t)
	ctx := context.Background()
	if _, err := a.RefreshSource(ctx, "nope"); err == nil || err.Error() != "source_not_found" {
		t.Errorf("RefreshSource = %v, want source_not_found", err)
	}
	if _, err := a.UpdateSource(ctx, "nope", library.NewSource{Name: "x"}); err == nil || err.Error() != "source_not_found" {
		t.Errorf("UpdateSource = %v, want source_not_found", err)
	}
}

func TestAddSourceValidation(t *testing.T) {
	a := newApp(t)
	ctx := context.Background()
	if _, err := a.AddSource(ctx, library.NewSource{Name: "  "}); err == nil || err.Error() != "name_required" {
		t.Errorf("blank name: %v, want name_required", err)
	}
	// A path the UI typed, not one from PickPlaylistFile, must be refused.
	req := library.NewSource{Name: "x", Kind: library.KindM3UFile, Path: filepath.Join(t.TempDir(), "list.m3u")}
	if _, err := a.AddSource(ctx, req); err == nil || err.Error() != "file_not_picked" {
		t.Errorf("unpicked file: %v, want file_not_picked", err)
	}
	if got := a.ListSources(); len(got) != 0 {
		t.Errorf("a refused source was stored: %v", got)
	}
}

func TestCurationCalls(t *testing.T) {
	a := newApp(t)
	if err := a.SetFavorite("c1", true); err != nil {
		t.Fatal(err)
	}
	if err := a.SetFavorite("c1", false); err != nil {
		t.Fatal(err)
	}
	if err := a.SetChannelHidden("c1", true); err != nil {
		t.Fatal(err)
	}
	if err := a.SetGroupHidden("News", true); err != nil {
		t.Fatal(err)
	}
	if got := a.ListHiddenGroups(); !slices.Equal(got, []string{"News"}) {
		t.Fatalf("hidden groups = %v", got)
	}
	if err := a.SetGroupHidden("News", false); err != nil {
		t.Fatal(err)
	}
	if got := a.ListHiddenGroups(); len(got) != 0 {
		t.Fatalf("hidden groups = %v", got)
	}
}

func TestLogoColorsEmpty(t *testing.T) {
	if got := newApp(t).LogoColors(context.Background(), nil); len(got) != 0 {
		t.Fatalf("LogoColors(nil) = %v", got)
	}
}
