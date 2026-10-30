package hostaddr

import (
	"context"
	"errors"
	"net"
	"net/netip"
	"runtime"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
)

// fakeDNS replaces the resolver for one test.
type fakeDNS struct {
	mu      sync.Mutex
	records map[string][]string
	err     error
	calls   map[string]int
}

func useFakeDNS(t *testing.T, records map[string][]string) *fakeDNS {
	t.Helper()
	f := &fakeDNS{records: records, calls: map[string]int{}}
	orig := lookup
	lookup = func(_ context.Context, host string) ([]netip.Addr, error) {
		f.mu.Lock()
		defer f.mu.Unlock()
		f.calls[host]++
		if f.err != nil {
			return nil, f.err
		}
		var out []netip.Addr
		for _, s := range f.records[host] {
			out = append(out, netip.MustParseAddr(s))
		}
		if len(out) == 0 {
			return nil, &net.DNSError{Err: "no such host", Name: host, IsNotFound: true}
		}
		return out, nil
	}
	t.Cleanup(func() {
		lookup = orig
		registryMu.Lock()
		clear(registry)
		registryMu.Unlock()
	})
	return f
}

func (f *fakeDNS) set(host string, addrs ...string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.records[host] = addrs
}

func (f *fakeDNS) fail(err error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.err = err
}

func TestGetResolvesAndMatches(t *testing.T) {
	useFakeDNS(t, map[string][]string{"dyn.example.com": {"203.0.113.7", "2001:db8::7"}})

	n, err := Get("Dyn.Example.com.")
	require.NoError(t, err)
	require.Equal(t, "dyn.example.com", n.Hostname())
	require.Equal(t, "host:dyn.example.com", n.String())

	require.True(t, n.Contains(net.ParseIP("203.0.113.7")))
	require.True(t, n.Contains(net.ParseIP("::ffff:203.0.113.7")))
	require.True(t, n.Contains(net.ParseIP("2001:db8::7")))
	require.False(t, n.Contains(net.ParseIP("203.0.113.8")))
	require.False(t, n.Contains(nil))
}

func TestGetSharesInstances(t *testing.T) {
	f := useFakeDNS(t, map[string][]string{"dyn.example.com": {"203.0.113.7"}})

	a, err := Get("dyn.example.com")
	require.NoError(t, err)
	b, err := Get("DYN.example.com")
	require.NoError(t, err)
	require.Same(t, a, b)
	require.Equal(t, 1, f.calls["dyn.example.com"])
}

func TestRefreshFollowsAddressChange(t *testing.T) {
	f := useFakeDNS(t, map[string][]string{"dyn.example.com": {"203.0.113.7"}})

	n, err := Get("dyn.example.com")
	require.NoError(t, err)
	gen := Generation()

	f.set("dyn.example.com", "198.51.100.9")
	refreshAll(t.Context())

	require.False(t, n.Contains(net.ParseIP("203.0.113.7")))
	require.True(t, n.Contains(net.ParseIP("198.51.100.9")))
	require.Greater(t, Generation(), gen)

	gen = Generation()
	refreshAll(t.Context())
	require.Equal(t, gen, Generation(), "unchanged addresses must not bump the generation")
}

func TestRefreshFailureKeepsLastAddresses(t *testing.T) {
	f := useFakeDNS(t, map[string][]string{"dyn.example.com": {"203.0.113.7"}})

	n, err := Get("dyn.example.com")
	require.NoError(t, err)

	f.fail(errors.New("dns down"))
	refreshAll(t.Context())
	require.True(t, n.Contains(net.ParseIP("203.0.113.7")))

	f.fail(nil)
	f.set("dyn.example.com")
	refreshAll(t.Context())
	require.True(t, n.Contains(net.ParseIP("203.0.113.7")), "NXDOMAIN keeps last addresses too")
}

func TestUnresolvedMatchesNothingUntilResolved(t *testing.T) {
	f := useFakeDNS(t, map[string][]string{})

	n, err := Get("later.example.com")
	require.NoError(t, err, "a failed first lookup must not fail config parsing")
	require.Nil(t, n.Addrs())
	require.False(t, n.Contains(net.ParseIP("203.0.113.7")))

	f.set("later.example.com", "203.0.113.7")
	refreshAll(t.Context())
	require.True(t, n.Contains(net.ParseIP("203.0.113.7")))
}

func TestUnreferencedNamesAreDropped(t *testing.T) {
	f := useFakeDNS(t, map[string][]string{"gone.example.com": {"203.0.113.7"}})

	_, err := Get("gone.example.com")
	require.NoError(t, err)
	runtime.GC()
	runtime.GC()
	refreshAll(t.Context())

	registryMu.Lock()
	_, ok := registry["gone.example.com"]
	registryMu.Unlock()
	require.False(t, ok)
	require.Equal(t, 1, f.calls["gone.example.com"], "dropped names are not looked up again")
}

func TestGetRejectsInvalidHostnames(t *testing.T) {
	useFakeDNS(t, map[string][]string{})
	for _, h := range []string{"", " ", "a..b", "-a.example.com", "a-.example.com", "a b.example.com", "a/24", "a:b"} {
		_, err := Get(h)
		require.ErrorIs(t, err, ErrInvalidHostname, h)
	}
}
