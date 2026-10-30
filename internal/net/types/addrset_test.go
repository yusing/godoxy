package nettypes

import (
	"net"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yusing/godoxy/internal/net/hostaddr"
)

func resetAddressSets(t *testing.T) {
	t.Helper()
	PublishAddressSets(nil)
	StageAddressSetNames(nil)
	t.Cleanup(func() {
		PublishAddressSets(nil)
		StageAddressSetNames(nil)
	})
}

func mustCIDRs(t *testing.T, entries ...string) []*CIDR {
	t.Helper()
	out := make([]*CIDR, len(entries))
	for i, e := range entries {
		out[i] = new(CIDR)
		require.NoError(t, out[i].Parse(e))
	}
	return out
}

func TestAddressSetReference(t *testing.T) {
	resetAddressSets(t)
	records := map[string][]string{"dino.dyndns.example": {"203.0.113.7"}}
	hostaddr.UseStaticRecordsForTest(t, records)

	var ref CIDR
	require.NoError(t, ref.Parse("set:family"))
	require.Equal(t, "set:family", ref.String())
	require.NotNil(t, ref.Set())
	require.False(t, ref.Set().Defined())
	require.False(t, ref.Contains(net.ParseIP("10.1.2.3")), "undefined set matches nothing")

	PublishAddressSets(map[string][]*CIDR{
		"family": mustCIDRs(t, "10.0.0.0/8", "host:dino.dyndns.example"),
	})
	require.True(t, ref.Set().Defined())
	require.True(t, ref.Contains(net.ParseIP("10.1.2.3")))
	require.True(t, ref.Contains(net.ParseIP("203.0.113.7")))
	require.False(t, ref.Contains(net.ParseIP("198.51.100.9")))

	// host: entries inside a set follow DNS changes
	records["dino.dyndns.example"] = []string{"198.51.100.9"}
	hostaddr.RefreshAllForTest(t)
	require.True(t, ref.Contains(net.ParseIP("198.51.100.9")))

	// a reload that redefines the set applies to existing references
	PublishAddressSets(map[string][]*CIDR{"family": mustCIDRs(t, "192.168.0.0/16")})
	require.False(t, ref.Contains(net.ParseIP("10.1.2.3")))
	require.True(t, ref.Contains(net.ParseIP("192.168.1.1")))

	// and one that removes it makes references match nothing
	PublishAddressSets(map[string][]*CIDR{})
	require.False(t, ref.Set().Defined())
	require.False(t, ref.Contains(net.ParseIP("192.168.1.1")))
}

func TestPublishAddressSetsCopiesInput(t *testing.T) {
	resetAddressSets(t)
	sets := map[string][]*CIDR{"office": mustCIDRs(t, "10.0.0.0/8")}
	PublishAddressSets(sets)
	sets["office"][0] = mustCIDRs(t, "192.168.0.0/16")[0]
	delete(sets, "office")

	ref, err := NewAddressSetRef("office")
	require.NoError(t, err)
	require.True(t, ref.Contains(net.ParseIP("10.1.2.3")))
}

func TestAddressSetNames(t *testing.T) {
	resetAddressSets(t)
	for _, name := range []string{"family", "Office_2", "a-b"} {
		_, err := NewAddressSetRef(name)
		require.NoError(t, err, name)
	}
	for _, bad := range []string{"set:", "set:a b", "set:a/b", "set:a:b", "set:a.b"} {
		var c CIDR
		require.Error(t, c.Parse(bad), bad)
	}
}

func TestAddressSetKnown(t *testing.T) {
	resetAddressSets(t)
	require.False(t, addressSetKnown("family"))
	StageAddressSetNames([]string{"family"})
	require.True(t, addressSetKnown("family"), "staged names count as known")
	StageAddressSetNames(nil)
	PublishAddressSets(map[string][]*CIDR{"family": nil})
	require.True(t, addressSetKnown("family"), "published names count as known")
}
