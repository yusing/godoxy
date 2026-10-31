# internal/net/hostaddr

Hostname address sources: `host:<name>` wherever GoDoxy accepts an IP or CIDR.

## Overview

A `host:<name>` entry matches the name's current A and AAAA addresses. It is
meant for clients or proxies behind dynamic DNS (for example a home router's
DDNS name), so an allow list follows their IP without a config reload.

Accepted in:

| Config | Example |
| --- | --- |
| `cidr_whitelist` `allow` | `- host:home.dyndns.example` |
| `real_ip` `from` | `- host:edge-proxy.example.com` |
| rules / bypass `remote` | `remote host:home.dyndns.example` |
| ACL `allow` / `deny` | `- host:home.dyndns.example` |
| access log `filters.cidr` | `- host:monitor.example.com` |

All entries except the ACL use `nettypes.CIDR` or the rules `remote`
validator, both of which call `Get`; the ACL has its own `host:` matcher
type, which calls `Get` too.

## Behavior

- **Resolution**: `net.DefaultResolver.LookupNetIP(ctx, "ip", name)`, so
  `/etc/hosts` and the system resolver configuration apply. Addresses are
  normalized (IPv4-mapped IPv6 unmapped, zones dropped, sorted, deduplicated)
  and compared exactly, like a bare IP (/32 or /128).
- **Refresh**: every referenced name is re-resolved every `RefreshInterval`
  (1 minute) by one background task. Lookups time out after `LookupTimeout`
  (5 seconds).
- **First lookup**: `Get` never waits for DNS. A new name starts its first
  lookup in its own goroutine, so the names of a config resolve in parallel
  and a DNS outage at startup neither stalls nor rejects the config. Until it
  resolves, the name matches nothing (fail closed for allow lists; for
  `real_ip` `from` the proxy is not trusted yet). `Name.Ready()` is closed
  once the first lookup finished.
- **Failures**: a failed first lookup is logged and retried every refresh. A
  later failure, including NXDOMAIN, keeps the last good addresses. Both are
  logged as warnings.
- **Changes**: logged at info level with old and new addresses. Matching
  reads an atomic pointer, so a change applies to the next request.
  `Generation()` increases on every change; the ACL adds it to its decision
  cache key when it has `host:` matchers.
- **Sharing and lifetime**: `Get` returns one shared `*Name` per normalized
  hostname. The registry holds weak pointers, so names no longer referenced
  by any loaded config are dropped on the next refresh.

## Public API

```go
const Prefix = "host:"

func Get(host string) (*Name, error) // validates, resolves, registers
func Generation() uint64

func (n *Name) Contains(ip net.IP) bool
func (n *Name) Ready() <-chan struct{} // closed after the first lookup
func (n *Name) Addrs() []netip.Addr
func (n *Name) Hostname() string
func (n *Name) String() string // "host:<name>"
```

Test helpers: `UseStaticRecordsForTest`, `RefreshAllForTest`.

## Security considerations

- A `host:` entry trusts whoever controls the name's DNS records and the
  resolver path. Prefer names under your control and a trusted resolver.
- Several names can share an address (CGNAT, shared hosting); allowing a name
  allows everyone behind its addresses.
- Until a `real_ip` `from` name first resolves (normally milliseconds after a
  load, longer if DNS is down), requests from that proxy keep its address. A
  whitelist that allows the proxy's own address admits them in that window;
  prefer allow lists that do not include the proxy.
- After an IP change, the old address stays allowed and the new one denied
  until a refresh succeeds: normally within a minute (plus any upstream DNS
  caching), but as long as DNS keeps failing, since a failed refresh keeps
  the last addresses.