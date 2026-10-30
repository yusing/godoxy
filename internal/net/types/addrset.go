package nettypes

import (
	"fmt"
	"net"
	"sync"
	"sync/atomic"

	"github.com/rs/zerolog/log"
)

// AddressSetPrefix marks a reference to a named address set, e.g. "set:family".
const AddressSetPrefix = "set:"

// AddressSet is a reference to a named set defined under the root config's
// address_sets. It holds only the name: every match reads the published
// definitions, so a reload that redefines the set applies to existing
// references, and a reload that is rejected changes nothing.
//
// An undefined set matches nothing.
type AddressSet struct {
	name string
}

var (
	// published definitions of the active config.
	addressSets atomic.Pointer[map[string][]*CIDR]

	// names defined by the config being loaded, used only to decide whether
	// a reference to a not-yet-published set deserves a warning.
	stagedMu    sync.Mutex
	stagedNames map[string]struct{}
)

// NewAddressSetRef validates name and returns a reference to it. It logs a
// warning, but does not fail, when no loaded or loading config defines it.
func NewAddressSetRef(name string) (*AddressSet, error) {
	if err := ValidateAddressSetName(name); err != nil {
		return nil, err
	}
	if !addressSetKnown(name) {
		log.Warn().Str("set", name).Msg("address set is not defined; references to it match nothing")
	}
	return &AddressSet{name: name}, nil
}

// ValidateAddressSetName checks that name is a non-empty identifier of
// letters, digits, '-' and '_'.
func ValidateAddressSetName(name string) error {
	if name == "" {
		return fmt.Errorf("invalid address set name %q", name)
	}
	for _, c := range name {
		if !(c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '-' || c == '_') {
			return fmt.Errorf("invalid address set name %q", name)
		}
	}
	return nil
}

// StageAddressSetNames records the set names of a config being loaded, so
// that references parsed before it is published do not warn.
func StageAddressSetNames(names []string) {
	staged := make(map[string]struct{}, len(names))
	for _, n := range names {
		staged[n] = struct{}{}
	}
	stagedMu.Lock()
	stagedNames = staged
	stagedMu.Unlock()
}

// PublishAddressSets makes sets the active definitions. Call it when a config
// is accepted, not while it is being prepared.
func PublishAddressSets(sets map[string][]*CIDR) {
	cloned := make(map[string][]*CIDR, len(sets))
	for name, cidrs := range sets {
		cloned[name] = append([]*CIDR(nil), cidrs...)
	}
	addressSets.Store(&cloned)
}

func addressSetKnown(name string) bool {
	if sets := addressSets.Load(); sets != nil {
		if _, ok := (*sets)[name]; ok {
			return true
		}
	}
	stagedMu.Lock()
	defer stagedMu.Unlock()
	_, ok := stagedNames[name]
	return ok
}

// Name returns the set name.
func (s *AddressSet) Name() string {
	return s.name
}

// String returns the reference form, "set:<name>".
func (s *AddressSet) String() string {
	return AddressSetPrefix + s.name
}

// Defined reports whether the active config defines the set.
func (s *AddressSet) Defined() bool {
	sets := addressSets.Load()
	if sets == nil {
		return false
	}
	_, ok := (*sets)[s.name]
	return ok
}

// Contains reports whether ip matches any entry of the set.
func (s *AddressSet) Contains(ip net.IP) bool {
	sets := addressSets.Load()
	if sets == nil {
		return false
	}
	for _, cidr := range (*sets)[s.name] {
		if cidr.Contains(ip) {
			return true
		}
	}
	return false
}
