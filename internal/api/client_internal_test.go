package api

import (
	"bytes"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"strings"
	"testing"
)

func TestClientAddr(t *testing.T) {
	t.Parallel()

	trusted := []netip.Prefix{netip.MustParsePrefix("10.0.0.0/8"), netip.MustParsePrefix("fd00::/8")}
	for _, tc := range []struct {
		name      string
		trusted   []netip.Prefix
		remote    string
		forwarded []string
		want      string
	}{
		{"no proxies trusted: the header is ignored", nil, "198.51.100.1:1", []string{"203.0.113.5"}, "198.51.100.1"},
		{"a peer that is no trusted proxy", trusted, "198.51.100.1:1", []string{"203.0.113.5"}, "198.51.100.1"},
		{"a trusted proxy", trusted, "10.0.0.2:1", []string{"203.0.113.5"}, "203.0.113.5"},
		{"what the client wrote to the left is ignored", trusted, "10.0.0.2:1", []string{"192.0.2.66, 203.0.113.5"}, "203.0.113.5"},
		{"a chain of trusted proxies", trusted, "10.0.0.2:1", []string{"203.0.113.5, 10.0.0.3"}, "203.0.113.5"},
		{"the header twice", trusted, "10.0.0.2:1", []string{"203.0.113.5", "10.0.0.3"}, "203.0.113.5"},
		{"only proxies", trusted, "10.0.0.2:1", []string{"10.0.0.3"}, "10.0.0.3"},
		{"a garbled entry stops the walk", trusted, "10.0.0.2:1", []string{"203.0.113.5, garbage"}, "10.0.0.2"},
		{"an IPv4-mapped peer", trusted, "[::ffff:10.0.0.2]:1", []string{"203.0.113.5"}, "203.0.113.5"},
		{"an IPv6 proxy", trusted, "[fd00::2]:1", []string{"2001:db8::7"}, "2001:db8::7"},
		{"a trusted proxy without the header", trusted, "10.0.0.2:1", nil, "10.0.0.2"},
		{"a peer address that does not parse", trusted, "pipe", nil, "invalid IP"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			c := &clients{trusted: tc.trusted, logger: slog.New(slog.DiscardHandler)}
			r := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/", http.NoBody)
			r.RemoteAddr = tc.remote
			for _, f := range tc.forwarded {
				r.Header.Add("X-Forwarded-For", f)
			}
			if got := c.addr(r).String(); got != tc.want {
				t.Errorf("addr = %s, want %s", got, tc.want)
			}
		})
	}
}

// Without trusted proxies, a forwarded request gets one warning in the
// log, not one per request.
func TestClientsWarnsOnceWithoutTrustedProxies(t *testing.T) {
	t.Parallel()

	var log bytes.Buffer
	c := &clients{logger: slog.New(slog.NewTextHandler(&log, nil))}
	for range 3 {
		r := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/", http.NoBody)
		r.Header.Set("X-Forwarded-For", "203.0.113.5")
		c.addr(r)
	}
	if n := strings.Count(log.String(), "TDEEMON_TRUSTED_PROXIES is not set"); n != 1 {
		t.Errorf("%d warnings, want 1:\n%s", n, log.String())
	}
}

func TestAddrKey(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct{ name, a, b string }{
		{"IPv6 by its /64", "2001:db8:1:2::1", "2001:db8:1:2:ffff::9"},
		{"IPv4 as it is", "203.0.113.5", "203.0.113.5"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if a, b := addrKey(netip.MustParseAddr(tc.a)), addrKey(netip.MustParseAddr(tc.b)); a != b {
				t.Errorf("keys %s and %s, want the same", a, b)
			}
		})
	}
	if addrKey(netip.MustParseAddr("2001:db8:1:2::1")) == addrKey(netip.MustParseAddr("2001:db8:1:3::1")) {
		t.Error("two /64s share a key")
	}
	if addrKey(netip.MustParseAddr("203.0.113.5")) == addrKey(netip.MustParseAddr("203.0.113.6")) {
		t.Error("two IPv4 addresses share a key")
	}
	if got := addrKey(netip.Addr{}); got != "unknown" {
		t.Errorf("addrKey of no address = %q, want unknown", got)
	}
}
