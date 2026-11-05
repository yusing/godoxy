package nettypes

import (
	"fmt"
	"net"
	"sync/atomic"
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
)

// NewAddressSetRef validates the name's syntax. Configuration acceptance checks
// its definition against the candidate, independently of the published sets.
func NewAddressSetRef(name string) (*AddressSet, error) {
	if err := ValidateAddressSetName(name); err != nil {
		return nil, err
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

// PublishAddressSets makes sets the active definitions. Call it when a config
// is accepted, not while it is being prepared.
func PublishAddressSets(sets map[string][]*CIDR) {
	cloned := make(map[string][]*CIDR, len(sets))
	for name, cidrs := range sets {
		cloned[name] = append([]*CIDR(nil), cidrs...)
	}
	addressSets.Store(&cloned)
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
