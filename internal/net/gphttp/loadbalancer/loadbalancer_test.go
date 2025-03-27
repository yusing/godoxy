package loadbalancer

import (
	"testing"

	"github.com/yusing/go-proxy/internal/net/gphttp/loadbalancer/types"
	. "github.com/yusing/go-proxy/internal/utils/testing"
)

func TestRebalance(t *testing.T) {
	t.Parallel()
	t.Run("zero", func(t *testing.T) {
		lb := New(new(types.Config))
		for range 10 {
			lb.AddServer(types.TestNewServer(0))
		}
		lb.rebalance()
		ExpectEqual(t, lb.sumWeight, maxWeight)
	})
	t.Run("less", func(t *testing.T) {
		lb := New(new(types.Config))
		lb.AddServer(types.TestNewServer(float64(maxWeight) * .1))
		lb.AddServer(types.TestNewServer(float64(maxWeight) * .2))
		lb.AddServer(types.TestNewServer(float64(maxWeight) * .3))
		lb.AddServer(types.TestNewServer(float64(maxWeight) * .2))
		lb.AddServer(types.TestNewServer(float64(maxWeight) * .1))
		lb.rebalance()
		// t.Logf("%s", U.Must(json.MarshalIndent(lb.pool, "", "  ")))
		ExpectEqual(t, lb.sumWeight, maxWeight)
	})
	t.Run("more", func(t *testing.T) {
		lb := New(new(types.Config))
		lb.AddServer(types.TestNewServer(float64(maxWeight) * .1))
		lb.AddServer(types.TestNewServer(float64(maxWeight) * .2))
		lb.AddServer(types.TestNewServer(float64(maxWeight) * .3))
		lb.AddServer(types.TestNewServer(float64(maxWeight) * .4))
		lb.AddServer(types.TestNewServer(float64(maxWeight) * .3))
		lb.AddServer(types.TestNewServer(float64(maxWeight) * .2))
		lb.AddServer(types.TestNewServer(float64(maxWeight) * .1))
		lb.rebalance()
		// t.Logf("%s", U.Must(json.MarshalIndent(lb.pool, "", "  ")))
		ExpectEqual(t, lb.sumWeight, maxWeight)
	})
}
