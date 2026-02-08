package route

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	route "github.com/yusing/godoxy/internal/route/types"
	"github.com/yusing/godoxy/internal/types"
)

func TestReverseProxyRoute(t *testing.T) {
	t.Run("LinkToLoadBalancer", func(t *testing.T) {
		cfg := Route{
			Alias:  "test",
			Scheme: route.SchemeHTTP,
			Host:   "example.com",
			Port:   Port{Proxy: 80},
			LoadBalance: &types.LoadBalancerConfig{
				Link: "test",
			},
		}
		cfg1 := Route{
			Alias:  "test1",
			Scheme: route.SchemeHTTP,
			Host:   "example.com",
			Port:   Port{Proxy: 80},
			LoadBalance: &types.LoadBalancerConfig{
				Link: "test",
			},
		}
		r, err := NewStartedTestRoute(t, &cfg)
		require.NoError(t, err)
		assert.NotNil(t, r)
		r2, err := NewStartedTestRoute(t, &cfg1)
		require.NoError(t, err)
		assert.NotNil(t, r2)
	})
}
