// Package ratelimit provides a Redis-backed token-bucket HTTP middleware
// and a RealIP helper that respects trusted-proxy CIDRs.
package ratelimit

import (
	"net"
	"net/http"
	"net/netip"
	"strings"
)

// RealIP returns the originating client IP for the request.
// If r.RemoteAddr falls inside trustedProxies, X-Forwarded-For is honoured
// (rightmost-trusted hops stripped, leftmost remaining hop returned).
// Otherwise RemoteAddr (host portion) is used.
func RealIP(r *http.Request, trustedProxies []netip.Prefix) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}
	addr, err := netip.ParseAddr(host)
	if err != nil {
		return host
	}

	if !inAny(addr, trustedProxies) {
		return host
	}

	xff := r.Header.Get("X-Forwarded-For")
	if xff == "" {
		return host
	}

	parts := strings.Split(xff, ",")
	// Walk right-to-left, skipping trusted hops; return first non-trusted hop.
	for i := len(parts) - 1; i >= 0; i-- {
		hop := strings.TrimSpace(parts[i])
		hopAddr, err := netip.ParseAddr(hop)
		if err != nil {
			continue
		}
		if !inAny(hopAddr, trustedProxies) {
			return hop
		}
	}
	return host
}

func inAny(addr netip.Addr, prefixes []netip.Prefix) bool {
	for _, p := range prefixes {
		if p.Contains(addr) {
			return true
		}
	}
	return false
}
