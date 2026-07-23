package uptime

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yusing/godoxy/internal/health"
)

func TestRouteAggregateUsesLatestSampleAsCurrentStatus(t *testing.T) {
	statuses := RouteStatuses{
		"route": {
			{Status: health.StatusHealthy, Timestamp: 1},
			{Status: health.StatusNapping, Timestamp: 2},
		},
		"empty": nil,
	}

	aggregated := statuses.aggregate(0, 0)
	require.Len(t, aggregated, 2)
	require.Equal(t, "empty", aggregated[0].Alias)
	require.Equal(t, health.StatusUnknown, aggregated[0].CurrentStatus)
	require.Equal(t, "route", aggregated[1].Alias)
	require.Equal(t, health.StatusNapping, aggregated[1].CurrentStatus)
}
