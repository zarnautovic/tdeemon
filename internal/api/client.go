package api

import (
	"context"
	"log/slog"
	"net/http"
	"net/netip"
	"slices"
	"strings"
	"sync/atomic"
)

// clientAddrKey is the context key of a request's client address.
type clientAddrKey struct{}

// clientAddrFrom returns the client address resolveClients put in ctx; the
// zero Addr if it could not tell.
func clientAddrFrom(ctx context.Context) netip.Addr {
	a, _ := ctx.Value(clientAddrKey{}).(netip.Addr)
	return a
}

// clients finds a request's client: the peer, or, when the peer is a
// trusted reverse proxy, the address it forwarded.
type clients struct {
	trusted []netip.Prefix
	logger  *slog.Logger
	// warned and warnedPeer make each warning go to the log once.
	warned, warnedPeer atomic.Bool
}

// addr returns r's client address. Behind trusted proxies it is the
// right-most address in X-Forwarded-For that is not one of them: the
// entries to its left come from the client and prove nothing.
func (c *clients) addr(r *http.Request) netip.Addr {
	peer, err := netip.ParseAddrPort(r.RemoteAddr)
	if err != nil {
		return netip.Addr{}
	}
	addr := peer.Addr().Unmap()
	forwarded := r.Header.Values("X-Forwarded-For")
	if len(c.trusted) == 0 {
		if len(forwarded) > 0 && !c.warned.Swap(true) {
			c.logger.WarnContext(r.Context(), "requests come through a proxy (X-Forwarded-For), but TDEEMON_TRUSTED_PROXIES is not set: "+
				"the limits on failed sign-ins count the proxy's address, that is everyone together", "peer", addr.String())
		}
		return addr
	}
	if !c.isTrusted(addr) {
		if len(forwarded) > 0 && !c.warnedPeer.Swap(true) {
			c.logger.WarnContext(r.Context(), "a request came with X-Forwarded-For from a peer that is not in TDEEMON_TRUSTED_PROXIES: "+
				"the header was ignored; if a proxy sent it, add its address", "peer", addr.String())
		}
		return addr
	}
	hops := strings.Split(strings.Join(forwarded, ","), ",")
	for _, hop := range slices.Backward(hops) {
		a, err := netip.ParseAddr(strings.TrimSpace(hop))
		if err != nil {
			return addr // A garbled entry: the last good one is as far as the proxies vouch.
		}
		addr = a.Unmap()
		if !c.isTrusted(addr) {
			return addr
		}
	}
	return addr
}

func (c *clients) isTrusted(a netip.Addr) bool {
	return slices.ContainsFunc(c.trusted, func(p netip.Prefix) bool { return p.Contains(a) })
}

// resolve puts the client address of each request into its context.
func (c *clients) resolve(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), clientAddrKey{}, c.addr(r))))
	})
}

// addrKey is the key a limit counts an address under: an IPv6 address by
// its /64, since one host gets a whole /64 to pick addresses from.
func addrKey(a netip.Addr) string {
	switch {
	case !a.IsValid():
		return "unknown"
	case a.Is6():
		p, _ := a.Prefix(64) // A /64 of an IPv6 address always exists.
		return p.String()
	}
	return a.String()
}
