package player

import (
	"slices"
	"testing"
)

// Same expectations as args_per_player in player.rs.
func TestArgsPerPlayer(t *testing.T) {
	mpv := known[0].args("a$b", "http://s/1", Options{})
	if mpv[2] != "--title=Herakleo · a$$b" || !slices.Equal(mpv[3:], []string{"--hwdec=auto-safe", "--", "http://s/1"}) {
		t.Fatalf("mpv args %q", mpv)
	}
	if vlc := known[1].args("t", "http://s/1", Options{}); !slices.Equal(vlc, []string{"--meta-title=Herakleo · t", "http://s/1"}) {
		t.Fatalf("vlc args %q", vlc)
	}
	if !Known("vlc") || Known("sh") {
		t.Fatal("allowlist broken")
	}
}

// Same expectations as options_per_player in player.rs.
func TestOptionsPerPlayer(t *testing.T) {
	o := Options{SoftwareDecoding: true, CacheSecs: 30, Volume: 70}
	mpv := known[0].args("t", "http://s/1", o)
	want := []string{"--hwdec=no", "--cache=yes", "--cache-secs=30", "--volume=70", "--", "http://s/1"}
	if !slices.Equal(mpv[3:], want) {
		t.Fatalf("mpv args %q", mpv)
	}
	vlc := known[1].args("t", "http://s/1", o)
	if !slices.Equal(vlc[1:], []string{"--avcodec-hw=none", "--network-caching=30000", "http://s/1"}) {
		t.Fatalf("vlc args %q", vlc)
	}
	if got := known[2].args("t", "http://s/1", o); !slices.Equal(got, []string{"http://s/1"}) {
		t.Fatalf("celluloid must ignore options, got %q", got)
	}
	for _, bad := range []Options{{CacheSecs: 7}, {Volume: 101}, {Volume: -1}} {
		if bad.Validate() == nil {
			t.Errorf("%+v should be rejected", bad)
		}
	}
	if o.Validate() != nil {
		t.Errorf("%+v should be valid", o)
	}
}
