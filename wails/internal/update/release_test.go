package update

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"unicode/utf8"
)

func TestNewer(t *testing.T) {
	tests := []struct {
		name string
		a, b string
		want bool
	}{
		{"patch bump", "0.4.5", "0.4.4", true},
		{"minor beats patch", "0.5.0", "0.4.99", true},
		{"major", "1.0.0", "0.9.9", true},
		{"numeric not lexical", "0.10.0", "0.9.0", true},
		{"equal", "0.4.4", "0.4.4", false},
		{"older", "0.4.3", "0.4.4", false},
		{"malformed new", "0.5", "0.4.4", false},
		{"malformed current", "0.5.0", "dev", false},
		{"prerelease suffix", "0.5.0-rc1", "0.4.4", false},
		{"leading v rejected", "v0.5.0", "0.4.4", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Newer(tt.a, tt.b); got != tt.want {
				t.Errorf("Newer(%q, %q) = %v, want %v", tt.a, tt.b, got, tt.want)
			}
		})
	}
}

// A version is never newer than itself, and of two distinct well-formed
// versions exactly one is newer.
func FuzzNewer(f *testing.F) {
	f.Add("0.4.4", "0.5.0")
	f.Add("1.0.0", "1.0.0")
	f.Add("0.10.0", "0.9.9")
	f.Add("x", "0.0.1")
	f.Fuzz(func(t *testing.T, a, b string) {
		if Newer(a, a) {
			t.Fatalf("Newer(%q, %q) is true", a, a)
		}
		if Newer(a, b) && Newer(b, a) {
			t.Fatalf("both %q and %q are newer than each other", a, b)
		}
		_, okA := parse(a)
		_, okB := parse(b)
		pa, _ := parse(a)
		pb, _ := parse(b)
		if okA && okB && pa != pb && Newer(a, b) == Newer(b, a) {
			t.Fatalf("%q vs %q: no order", a, b)
		}
	})
}

func serveJSON(t *testing.T, body string) string {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)
	return srv.URL
}

func TestLatest(t *testing.T) {
	url := serveJSON(t, `{"tag_name":"v0.5.0","body":"### Features\n- updates","assets":[{"name":"SHA256SUMS"},{"name":"SHA256SUMS.sig"}]}`)
	r, err := Latest(context.Background(), http.DefaultClient, url)
	if err != nil {
		t.Fatal(err)
	}
	if r.Version != "0.5.0" || r.Notes == "" || !r.HasAsset(SigName) || r.HasAsset("nope") {
		t.Fatalf("got %+v", r)
	}
}

func TestLatestCutsNotesOnARuneBoundary(t *testing.T) {
	notes := strings.Repeat("a", maxNotes-1) + "é and more"
	body, _ := json.Marshal(map[string]string{"tag_name": "v0.5.0", "body": notes})
	r, err := Latest(context.Background(), http.DefaultClient, serveJSON(t, string(body)))
	if err != nil {
		t.Fatal(err)
	}
	if !utf8.ValidString(r.Notes) || len(r.Notes) > maxNotes {
		t.Fatalf("notes: valid=%v len=%d", utf8.ValidString(r.Notes), len(r.Notes))
	}
}

func TestLatestRejects(t *testing.T) {
	tests := map[string]string{
		"odd tag":        `{"tag_name":"v0.5.0/../../x"}`,
		"no v":           `{"tag_name":"0.5.0"}`,
		"prerelease":     `{"tag_name":"v0.5.0","prerelease":true}`,
		"draft":          `{"tag_name":"v0.5.0","draft":true}`,
		"not json":       `<html>`,
		"tag with query": `{"tag_name":"v0.5.0?x=1"}`,
	}
	for name, body := range tests {
		t.Run(name, func(t *testing.T) {
			if _, err := Latest(context.Background(), http.DefaultClient, serveJSON(t, body)); err == nil {
				t.Fatal("accepted")
			}
		})
	}
}
