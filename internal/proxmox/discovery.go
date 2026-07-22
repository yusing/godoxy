package proxmox

type DiscoveryKind string

const (
	DiscoveryNode     DiscoveryKind = "node"
	DiscoveryResource DiscoveryKind = "resource"
)

// Discovery is the allowlisted, non-secret result of resolving one route
// against the Proxmox inventory.
type Discovery struct {
	Kind   DiscoveryKind
	Node   string
	Alias  string
	VMID   uint64
	VMName string
	Target string
}
