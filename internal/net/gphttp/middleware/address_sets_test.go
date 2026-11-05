package middleware

import (
	"testing"

	"github.com/stretchr/testify/require"
	nettypes "github.com/yusing/godoxy/internal/net/types"
)

func TestAddressSetValidationFollowsComposedChains(t *testing.T) {
	guard := registerComposeChain(t, "address-set-guard", []map[string]any{
		{"use": "cidr_whitelist", "allow": []string{"set:family"}},
	})
	nested := registerComposeChain(t, "address-set-nested", []map[string]any{{"use": guard}})
	defs := map[string]OptionsRaw{nested: {}}
	require.ErrorIs(t, ValidateAddressSetMap(defs, nettypes.PublishedAddressSetValidator), nettypes.ErrUndefinedAddressSet)
	defs["invalid-middleware"] = OptionsRaw{}
	require.ErrorIs(t, ValidateAddressSetMap(defs, nettypes.PublishedAddressSetValidator), nettypes.ErrUndefinedAddressSet)
	delete(defs, "invalid-middleware")
	require.NoError(t, ValidateAddressSetMap(defs, func(name string) error {
		require.Equal(t, "family", name)
		return nil
	}))
}
