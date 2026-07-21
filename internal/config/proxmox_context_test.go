package config

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yusing/godoxy/internal/proxmox"
)

func TestNewStateOwnsAnIsolatedProxmoxNodePool(t *testing.T) {
	first := NewState()
	second := NewState()
	t.Cleanup(func() {
		first.Task().Finish(nil)
		second.Task().Finish(nil)
	})

	firstNodes := proxmox.FromCtx(first.Context())
	secondNodes := proxmox.FromCtx(second.Context())
	require.NotNil(t, firstNodes)
	require.NotNil(t, secondNodes)
	require.NotSame(t, firstNodes, secondNodes)
}
