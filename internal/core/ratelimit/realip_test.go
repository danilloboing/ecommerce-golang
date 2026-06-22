package ratelimit_test

import (
	"net/http/httptest"
	"net/netip"
	"testing"

	"github.com/danilloboing/marketplace-golang/internal/core/ratelimit"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func parseCIDRs(t *testing.T, raw ...string) []netip.Prefix {
	t.Helper()
	out := make([]netip.Prefix, 0, len(raw))
	for _, s := range raw {
		p, err := netip.ParsePrefix(s)
		require.NoError(t, err)
		out = append(out, p)
	}
	return out
}

func TestRealIP_UsesRemoteAddrWhenNoTrustedProxy(t *testing.T) {
	r := httptest.NewRequest("GET", "/", nil)
	r.RemoteAddr = "8.8.8.8:1234"
	r.Header.Set("X-Forwarded-For", "1.2.3.4, 5.6.7.8")

	got := ratelimit.RealIP(r, nil)
	assert.Equal(t, "8.8.8.8", got)
}

func TestRealIP_ReturnsClientHopWhenComingFromTrustedProxy(t *testing.T) {
	trusted := parseCIDRs(t, "10.0.0.0/8")
	r := httptest.NewRequest("GET", "/", nil)
	r.RemoteAddr = "10.1.2.3:1234"
	r.Header.Set("X-Forwarded-For", "1.2.3.4, 10.1.2.4")

	got := ratelimit.RealIP(r, trusted)
	assert.Equal(t, "1.2.3.4", got)
}

func TestRealIP_FallsBackToRemoteAddrWhenXFFEmpty(t *testing.T) {
	trusted := parseCIDRs(t, "10.0.0.0/8")
	r := httptest.NewRequest("GET", "/", nil)
	r.RemoteAddr = "10.1.2.3:1234"

	got := ratelimit.RealIP(r, trusted)
	assert.Equal(t, "10.1.2.3", got)
}
