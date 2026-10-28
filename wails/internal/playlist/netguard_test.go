package playlist

import (
	"context"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"testing"
)

func TestRedirectIntoPrivateAddressBlocked(t *testing.T) {
	// httptest only listens on loopback, so pretend 127.0.0.1 is a public
	// server and put the "LAN" target on 127.0.0.2.
	orig := privateAddr
	privateAddr = func(a netip.Addr) bool { return a != netip.MustParseAddr("127.0.0.1") && orig(a) }
	defer func() { privateAddr = orig }()

	ln, err := net.Listen("tcp", "127.0.0.2:0")
	if err != nil {
		t.Skip("127.0.0.2 not available:", err)
	}
	lan := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, "#EXTM3U\n#EXTINF:-1,A\nhttp://s/a\n")
	}))
	_ = lan.Listener.Close()
	lan.Listener = ln
	lan.Start()
	defer lan.Close()

	public := httptest.NewServer(http.RedirectHandler(lan.URL+"/list.m3u", http.StatusFound))
	defer public.Close()

	_, err = M3UFromURL(context.Background(), public.URL)
	if err == nil || err.Error() != "download_failed: private_address_blocked" {
		t.Fatalf("redirect from public host: got %v", err)
	}
	// A source the user points at the LAN directly keeps working.
	got, err := M3UFromURL(context.Background(), lan.URL+"/list.m3u")
	if err != nil || len(got.Entries) != 1 {
		t.Fatalf("direct LAN URL: %+v, %v", got, err)
	}
}

func TestPrivateAddr(t *testing.T) {
	for addr, want := range map[string]bool{
		"127.0.0.1": true, "10.1.2.3": true, "192.168.1.1": true, "172.16.0.1": true,
		"169.254.169.254": true, "0.0.0.0": true, "::1": true, "fd00::1": true, "fe80::1": true,
		"::ffff:127.0.0.1": true, "8.8.8.8": false, "2606:4700::1111": false,
	} {
		if got := privateAddr(netip.MustParseAddr(addr)); got != want {
			t.Errorf("privateAddr(%s) = %v, want %v", addr, got, want)
		}
	}
}
