// Package hostaddr resolves hostnames used as address sources ("host:name" in
// cidr_whitelist, real_ip, ACL, rules `remote` and access log filters) and keeps
// their addresses fresh in the background, so a dynamic DNS name follows IP
// changes without a config reload.
package hostaddr

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/netip"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"time"
	"weak"

	"github.com/rs/zerolog/log"
	"github.com/yusing/goutils/task"
)

// Prefix marks a hostname address source, e.g. "host:example.dyndns.org".
const Prefix = "host:"

var (
	// RefreshInterval is how often every referenced hostname is re-resolved.
	RefreshInterval = time.Minute
	// LookupTimeout bounds a single A/AAAA lookup.
	LookupTimeout = 5 * time.Second

	// lookup resolves A and AAAA records; replaced in tests.
	lookup = func(ctx context.Context, host string) ([]netip.Addr, error) {
		return net.DefaultResolver.LookupNetIP(ctx, "ip", host)
	}
)

var ErrInvalidHostname = errors.New("invalid hostname")

// Name is a hostname whose A/AAAA addresses are re-resolved every
// RefreshInterval. Until the first successful lookup (started by Get in the
// background) it matches nothing; after that a failed lookup keeps the last
// good addresses.
type Name struct {
	name  string
	addrs atomic.Pointer[[]netip.Addr]
	// ready is closed once the first lookup has finished, successful or not.
	ready chan struct{}
}

var (
	registryMu sync.Mutex
	registry   = make(map[string]weak.Pointer[Name])
	startLoop  sync.Once

	// generation increases whenever any name resolves to a different set of
	// addresses, so callers can invalidate caches keyed by IP.
	generation atomic.Uint64

	// syncFirstLookup makes Get resolve new names before returning; only set
	// by UseStaticRecordsForTest, so consumers' tests can assert right after
	// parsing.
	syncFirstLookup atomic.Bool
)

// Get returns the shared Name for host. It never waits for DNS: a new name
// starts its first lookup in its own goroutine (so the names of a config
// resolve in parallel) and matches nothing until that lookup succeeds. A
// failed first lookup is logged and retried every RefreshInterval; it does
// not make Get fail, so a DNS outage at startup neither rejects nor stalls
// the config. Entries are dropped once no caller references them.
func Get(host string) (*Name, error) {
	host = strings.TrimSuffix(strings.ToLower(strings.TrimSpace(host)), ".")
	if err := validateHostname(host); err != nil {
		return nil, err
	}

	registryMu.Lock()
	if p, ok := registry[host]; ok {
		if n := p.Value(); n != nil {
			registryMu.Unlock()
			return n, nil
		}
	}
	n := &Name{name: host, ready: make(chan struct{})}
	registry[host] = weak.Make(n)
	registryMu.Unlock()

	startLoop.Do(startRefreshLoop)
	if syncFirstLookup.Load() {
		n.firstLookup()
	} else {
		go n.firstLookup()
	}
	return n, nil
}

func (n *Name) firstLookup() {
	n.refresh(task.RootContext())
	close(n.ready)
}

// Ready is closed once the first lookup has finished, successful or not.
func (n *Name) Ready() <-chan struct{} {
	return n.ready
}

// Generation returns a counter that increases on every address change of any
// Name.
func Generation() uint64 {
	return generation.Load()
}

// Hostname returns the normalized hostname.
func (n *Name) Hostname() string {
	return n.name
}

// String returns the source form, "host:<name>".
func (n *Name) String() string {
	return Prefix + n.name
}

// Addrs returns the current addresses, nil before the first successful lookup.
func (n *Name) Addrs() []netip.Addr {
	if p := n.addrs.Load(); p != nil {
		return *p
	}
	return nil
}

// Contains reports whether ip is one of the current addresses.
func (n *Name) Contains(ip net.IP) bool {
	addr, ok := netip.AddrFromSlice(ip)
	if !ok {
		return false
	}
	addr = addr.Unmap()
	for _, a := range n.Addrs() {
		if a == addr {
			return true
		}
	}
	return false
}

func (n *Name) refresh(parent context.Context) {
	ctx, cancel := context.WithTimeout(parent, LookupTimeout)
	defer cancel()

	addrs, err := lookup(ctx, n.name)
	if err == nil && len(addrs) == 0 {
		err = errors.New("no addresses")
	}
	old := n.Addrs()
	if err != nil {
		if old == nil {
			log.Warn().Err(err).Str("host", n.name).Msg("hostname source unresolved, matching nothing until it resolves")
		} else {
			log.Warn().Err(err).Str("host", n.name).Strs("addrs", addrStrings(old)).Msg("hostname source lookup failed, keeping last addresses")
		}
		return
	}

	addrs = normalize(addrs)
	if slices.Equal(addrs, old) {
		return
	}
	n.addrs.Store(&addrs)
	generation.Add(1)
	if old == nil {
		log.Info().Str("host", n.name).Strs("addrs", addrStrings(addrs)).Msg("hostname source resolved")
	} else {
		log.Info().Str("host", n.name).Strs("old", addrStrings(old)).Strs("new", addrStrings(addrs)).Msg("hostname source addresses changed")
	}
}

func startRefreshLoop() {
	t := task.RootTask("hostaddr_refresh", false)
	go func() {
		ticker := time.NewTicker(RefreshInterval)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				refreshAll(t.Context())
			case <-t.Context().Done():
				return
			}
		}
	}()
}

// refreshAll re-resolves every live Name concurrently and drops entries that
// are no longer referenced.
func refreshAll(ctx context.Context) {
	var live []*Name
	registryMu.Lock()
	for host, p := range registry {
		if n := p.Value(); n != nil {
			live = append(live, n)
		} else {
			delete(registry, host)
		}
	}
	registryMu.Unlock()

	var wg sync.WaitGroup
	for _, n := range live {
		wg.Go(func() { n.refresh(ctx) })
	}
	wg.Wait()
}

func normalize(addrs []netip.Addr) []netip.Addr {
	out := make([]netip.Addr, 0, len(addrs))
	for _, a := range addrs {
		out = append(out, a.Unmap().WithZone(""))
	}
	slices.SortFunc(out, func(a, b netip.Addr) int { return a.Compare(b) })
	return slices.Compact(out)
}

func addrStrings(addrs []netip.Addr) []string {
	s := make([]string, len(addrs))
	for i, a := range addrs {
		s[i] = a.String()
	}
	return s
}

func validateHostname(host string) error {
	if host == "" || len(host) > 253 {
		return fmt.Errorf("%w: %q", ErrInvalidHostname, host)
	}
	for label := range strings.SplitSeq(host, ".") {
		if label == "" || len(label) > 63 || label[0] == '-' || label[len(label)-1] == '-' {
			return fmt.Errorf("%w: %q", ErrInvalidHostname, host)
		}
		for _, c := range label {
			if !(c >= 'a' && c <= 'z' || c >= '0' && c <= '9' || c == '-' || c == '_') {
				return fmt.Errorf("%w: %q", ErrInvalidHostname, host)
			}
		}
	}
	return nil
}
