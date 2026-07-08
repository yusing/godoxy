package loadbalancer

import (
	"testing"

	"github.com/yusing/godoxy/internal/loadbalancer"
	expect "github.com/yusing/goutils/testing"
)

func TestRebalance(t *testing.T) {
	t.Parallel()
	t.Run("zero", func(t *testing.T) {
		lb := New(new(loadbalancer.Config))
		for range 10 {
			lb.AddServer(newTestServer(0))
		}
		lb.rebalance()
		expect.Equal(t, lb.sumWeight, maxWeight)
	})
	t.Run("less", func(t *testing.T) {
		lb := New(new(loadbalancer.Config))
		lb.AddServer(newTestServer(float64(maxWeight) * .1))
		lb.AddServer(newTestServer(float64(maxWeight) * .2))
		lb.AddServer(newTestServer(float64(maxWeight) * .3))
		lb.AddServer(newTestServer(float64(maxWeight) * .2))
		lb.AddServer(newTestServer(float64(maxWeight) * .1))
		lb.rebalance()
		// t.Logf("%s", U.Must(json.MarshalIndent(lb.pool, "", "  ")))
		expect.Equal(t, lb.sumWeight, maxWeight)
	})
	t.Run("more", func(t *testing.T) {
		lb := New(new(loadbalancer.Config))
		lb.AddServer(newTestServer(float64(maxWeight) * .1))
		lb.AddServer(newTestServer(float64(maxWeight) * .2))
		lb.AddServer(newTestServer(float64(maxWeight) * .3))
		lb.AddServer(newTestServer(float64(maxWeight) * .4))
		lb.AddServer(newTestServer(float64(maxWeight) * .3))
		lb.AddServer(newTestServer(float64(maxWeight) * .2))
		lb.AddServer(newTestServer(float64(maxWeight) * .1))
		lb.rebalance()
		// t.Logf("%s", U.Must(json.MarshalIndent(lb.pool, "", "  ")))
		expect.Equal(t, lb.sumWeight, maxWeight)
	})
}
