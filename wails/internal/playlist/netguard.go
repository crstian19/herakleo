package playlist

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"syscall"
	"time"
)

// errPrivateAddress is the detail of download_failed when a public source
// ends up dialling a private address.
var errPrivateAddress = errors.New("private_address_blocked")

// privateAddr reports addresses a public server must not steer us to:
// loopback, RFC 1918 / ULA, link-local (cloud metadata lives at 169.254.169.254)
// and unspecified. A variable so tests can treat 127.0.0.1 as public.
var privateAddr = func(a netip.Addr) bool {
	a = a.Unmap()
	return a.IsLoopback() || a.IsPrivate() || a.IsLinkLocalUnicast() ||
		a.IsLinkLocalMulticast() || a.IsInterfaceLocalMulticast() || a.IsUnspecified()
}

// Clients pairs an unrestricted client with one that refuses to connect to
// private addresses. The check runs in the dialer on the IP actually dialled,
// so it covers redirects and DNS rebinding alike, and the two never share a
// connection pool, so a LAN connection cannot be reused for a public source.
type Clients struct {
	public, private *http.Client
}

// NewClients builds both clients with the same timeout.
func NewClients(timeout time.Duration) Clients {
	guarded := http.DefaultTransport.(*http.Transport).Clone()
	dialer := &net.Dialer{
		Timeout:   30 * time.Second,
		KeepAlive: 30 * time.Second,
		Control: func(_, address string, _ syscall.RawConn) error {
			ap, err := netip.ParseAddrPort(address)
			if err != nil || privateAddr(ap.Addr()) {
				return errPrivateAddress
			}
			return nil
		},
	}
	guarded.DialContext = dialer.DialContext
	// The dialler only sees the proxy, so a proxied request could not be
	// checked anyway; and blocking a proxy on localhost would break it.
	guarded.Proxy = nil
	return Clients{
		public:  &http.Client{Timeout: timeout, Transport: guarded},
		private: &http.Client{Timeout: timeout, Transport: http.DefaultTransport.(*http.Transport).Clone()},
	}
}

// For picks the client for a request to u. Users run panels and proxies on
// their LAN, so a source that is itself private may go anywhere; a public
// one may not be redirected or rebound into the LAN.
//
// With resolve, a host name counts as private when it resolves to a private
// address (panel.lan); a provider controlling that DNS could point it at
// localhost, but the user chose to trust that name. Without resolve only
// literal IPs and localhost count, for URLs the user never typed (logos).
// A configured HTTP proxy is honoured only on the unrestricted client.
func (c Clients) For(ctx context.Context, u *url.URL, resolve bool) *http.Client {
	if privateHost(ctx, u.Hostname(), resolve) {
		return c.private
	}
	if p, err := http.ProxyFromEnvironment(&http.Request{URL: u}); err == nil && p != nil {
		return c.private
	}
	return c.public
}

func privateHost(ctx context.Context, host string, resolve bool) bool {
	if host == "localhost" {
		return true
	}
	if a, err := netip.ParseAddr(host); err == nil {
		return privateAddr(a)
	}
	if !resolve {
		return false
	}
	addrs, err := net.DefaultResolver.LookupNetIP(ctx, "ip", host)
	if err != nil {
		return false // the request fails on its own, through the guarded client
	}
	for _, a := range addrs {
		if privateAddr(a) {
			return true
		}
	}
	return false
}
