package hostaddr

import (
	"context"
	"net/netip"
	"runtime"
	"testing"
)

// UseStaticRecordsForTest makes lookups return records (host -> addresses)
// until the test ends, and resets the registry around it. Mutating records
// and calling RefreshAllForTest simulates a DNS change. Fixture names are loaded
// through Get and awaited before returning, so consumers can assert after parsing.
func UseStaticRecordsForTest(tb testing.TB, records map[string][]string) {
	tb.Helper()
	orig := lookup
	resetRegistry()
	lookup = func(_ context.Context, host string) ([]netip.Addr, error) {
		var out []netip.Addr
		for _, s := range records[host] {
			out = append(out, netip.MustParseAddr(s))
		}
		return out, nil
	}
	var names []*Name
	tb.Cleanup(func() {
		runtime.KeepAlive(names)
		lookup = orig
		resetRegistry()
	})
	for host := range records {
		name, err := Get(host)
		if err != nil {
			tb.Fatal(err)
		}
		<-name.Ready()
		names = append(names, name)
	}
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
