package library

import (
	"encoding/json"
	"fmt"
	"os"
	"slices"
	"testing"
	"time"
)

// TestBenchRealLibrary times the hot paths on a real library.
// It only runs when HERAKLEO_BENCH_DIR points at a library directory (use a copy).
func TestBenchRealLibrary(t *testing.T) {
	dir := os.Getenv("HERAKLEO_BENCH_DIR")
	if dir == "" {
		t.Skip("HERAKLEO_BENCH_DIR not set")
	}
	measure := func(name string, runs int, f func()) {
		f() // warm-up
		d := make([]time.Duration, runs)
		for i := range d {
			start := time.Now()
			f()
			d[i] = time.Since(start)
		}
		slices.Sort(d)
		ms := func(x time.Duration) float64 { return float64(x.Microseconds()) / 1000 }
		fmt.Printf("BENCH go %-14s median %8.2f ms  p95 %8.2f ms\n", name, ms(d[len(d)/2]), ms(d[len(d)*95/100]))
	}

	var l *Library
	measure("open", 5, func() {
		var err error
		if l, err = Open(dir); err != nil {
			t.Fatal(err)
		}
	})
	group := l.Groups("")[0].Name
	query := func(q Query) func() { return func() { l.Channels(q) } }
	measure("page", 50, query(Query{Limit: 120}))
	measure("page+json", 50, func() { _, _ = json.Marshal(l.Channels(Query{Limit: 120})) })
	measure("search_common", 50, query(Query{Search: "es", Limit: 120}))
	measure("search_rare", 50, query(Query{Search: "zzqx", Limit: 120}))
	measure("group", 50, query(Query{Group: group, Limit: 120}))
	measure("sort_az", 50, query(Query{Sort: "az", Limit: 120}))
	measure("groups", 50, func() { l.Groups("") })
}
