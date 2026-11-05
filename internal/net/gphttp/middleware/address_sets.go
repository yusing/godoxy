package middleware

import (
	"errors"

	nettypes "github.com/yusing/godoxy/internal/net/types"
)

func (m *Middleware) ValidateAddressSetReferences(check nettypes.AddressSetValidator) error {
	if err := nettypes.ValidateAddressSetReferences(m.Bypass, check); err != nil {
		return err
	}
	return nettypes.ValidateAddressSetReferences(m.impl, check)
}

func (m *checkBypass) ValidateAddressSetReferences(check nettypes.AddressSetValidator) error {
	for _, value := range []any{m.bypass, m.modReq, m.modRes} {
		if err := nettypes.ValidateAddressSetReferences(value, check); err != nil {
			return err
		}
	}
	return nil
}

func (m *middlewareChain) ValidateAddressSetReferences(check nettypes.AddressSetValidator) error {
	for _, value := range []any{m.befores, m.respHeader, m.respBody} {
		if err := nettypes.ValidateAddressSetReferences(value, check); err != nil {
			return err
		}
	}
	return nil
}

// ValidateAddressSetMap also follows composed middleware loaded from files.
func ValidateAddressSetMap(defs map[string]OptionsRaw, check nettypes.AddressSetValidator) error {
	compiled, err := compileMiddlewares(defs)
	return errors.Join(err, nettypes.ValidateAddressSetReferences(compiled, check))
}
