package hostaddr

import (
	"context"
	"net/netip"
	"testing"
)

// UseStaticRecordsForTest makes lookups return records (host -> addresses)
// until the test ends, and resets the registry around it. Mutating records
// and calling RefreshAllForTest simulates a DNS change. While it is active,
// Get resolves new names before returning, so tests can assert right after
// parsing.
func UseStaticRecordsForTest(tb testing.TB, records map[string][]string) {
	tb.Helper()
	orig := lookup
	resetRegistry()
	syncFirstLookup.Store(true)
	lookup = func(_ context.Context, host string) ([]netip.Addr, error) {
		var out []netip.Addr
		for _, s := range records[host] {
			out = append(out, netip.MustParseAddr(s))
		}
		return out, nil
	}
	tb.Cleanup(func() {
		lookup = orig
		syncFirstLookup.Store(false)
		resetRegistry()
	})
}

// RefreshAllForTest re-resolves every referenced name now.
func RefreshAllForTest(tb testing.TB) {
	tb.Helper()
	refreshAll(context.Background())
}

func resetRegistry() {
	registryMu.Lock()
	clear(registry)
	registryMu.Unlock()
}
