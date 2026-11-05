package rules

import nettypes "github.com/yusing/godoxy/internal/net/types"

// Validate the same parsed source that matching uses, including negation.
func (c onMatcher) ValidateAddressSetReferences(check nettypes.AddressSetValidator) error {
	return nettypes.ValidateAddressSetReferences(c.set, check)
}

func (on RuleOn) ValidateAddressSetReferences(check nettypes.AddressSetValidator) error {
	return nettypes.ValidateAddressSetReferences(on.checker, check)
}

func (cmd Command) ValidateAddressSetReferences(check nettypes.AddressSetValidator) error {
	if err := nettypes.ValidateAddressSetReferences(cmd.pre, check); err != nil {
		return err
	}
	return nettypes.ValidateAddressSetReferences(cmd.post, check)
}

func (h Handler) ValidateAddressSetReferences(check nettypes.AddressSetValidator) error {
	if h.validateAddressSets != nil {
		return h.validateAddressSets(check)
	}
	return nil
}

// A middleware action resolves outside this package to avoid an import cycle.
// Its validator is bound to the constructed instance, not a global candidate.
type ResolvedRequestMiddleware struct {
	Run                 RequestMiddleware
	ValidateAddressSets func(nettypes.AddressSetValidator) error
}

func addressSetValidation(args any) func(nettypes.AddressSetValidator) error {
	if m, ok := args.(ResolvedRequestMiddleware); ok {
		return m.ValidateAddressSets
	}
	return nil
}
