package config

import (
	"net"
	"testing"

	"github.com/stretchr/testify/require"
	config "github.com/yusing/godoxy/internal/config/types"
	"github.com/yusing/godoxy/internal/net/hostaddr"
	nettypes "github.com/yusing/godoxy/internal/net/types"
)

const addressSetsConfig = `
address_sets:
  family:
    - 10.0.0.0/8
    - host:localhost
acl:
  default: deny
  allow:
    - set:family
entrypoint:
  middlewares:
    - use: cidr_whitelist
      allow:
        - set:family
`

func resetPublishedAddressSets(t *testing.T) {
	t.Helper()
	// host: entries resolve in the background; pin them so assertions
	// right after loading are deterministic.
	hostaddr.UseStaticRecordsForTest(t, map[string][]string{"localhost": {"127.0.0.1"}})
	nettypes.PublishAddressSets(nil)
	t.Cleanup(func() { nettypes.PublishAddressSets(nil) })
}

func TestAddressSetsPublishedOnlyWhenAccepted(t *testing.T) {
	resetPublishedAddressSets(t)

	state := NewState()
	t.Cleanup(func() { state.Stop(nil) })
	require.NoError(t, state.Init([]byte(addressSetsConfig)))
	require.Len(t, state.Value().AddressSets["family"], 2)

	ref, err := nettypes.NewAddressSetRef("family")
	require.NoError(t, err)
	require.False(t, ref.Defined(), "a prepared but not accepted config must not publish its sets")

	state.setStatus(config.RuntimeActivating)
	require.True(t, ref.Defined())
	require.True(t, ref.Contains(net.ParseIP("10.1.2.3")))
	require.True(t, ref.Contains(net.ParseIP("127.0.0.1")), "host: entries resolve inside sets")
	require.False(t, ref.Contains(net.ParseIP("203.0.113.7")))
}

func TestAddressSetsRejectedReloadKeepsActiveSets(t *testing.T) {
	resetPublishedAddressSets(t)

	active := NewState()
	t.Cleanup(func() { active.Stop(nil) })
	require.NoError(t, active.Init([]byte(addressSetsConfig)))
	active.setStatus(config.RuntimeActivating)

	candidate := NewState()
	t.Cleanup(func() { candidate.Stop(nil) })
	err := candidate.Init([]byte("address_sets:\n  family:\n    - set:other\n  other:\n    - 192.168.0.0/16\n"))
	require.ErrorContains(t, err, "address sets cannot reference other sets")

	ref, err := nettypes.NewAddressSetRef("family")
	require.NoError(t, err)
	require.True(t, ref.Contains(net.ParseIP("10.1.2.3")))
	require.False(t, ref.Contains(net.ParseIP("192.168.1.1")))
}

func TestAddressSetsValidation(t *testing.T) {
	resetPublishedAddressSets(t)

	for name, doc := range map[string]string{
		"nested set": "address_sets:\n  a:\n    - set:b\n  b:\n    - 10.0.0.0/8\n",
		"bad name":   "address_sets:\n  bad.name:\n    - 10.0.0.0/8\n",
		"bad entry":  "address_sets:\n  a:\n    - not-an-ip\n",
	} {
		t.Run(name, func(t *testing.T) {
			require.Error(t, config.Validate([]byte(doc)))
		})
	}
	require.NoError(t, config.Validate([]byte(addressSetsConfig)))
}
