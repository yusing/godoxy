package uptime

import (
	"context"
	"encoding/json"
	"net/url"
	"slices"
	"time"

	"github.com/lithammer/fuzzysearch/fuzzy"
	config "github.com/yusing/godoxy/internal/config/types"
	"github.com/yusing/godoxy/internal/metrics/period"
	metricsutils "github.com/yusing/godoxy/internal/metrics/utils"
	"github.com/yusing/godoxy/internal/route/routes"
	"github.com/yusing/godoxy/internal/types"
)

type (
	StatusByAlias struct {
		Map       map[string]*routes.HealthInfo `json:"statuses"`
		Timestamp int64                         `json:"timestamp"`
	} // @name RouteStatusesByAlias
	Status struct {
		Status    types.HealthStatus `json:"status" swaggertype:"string" enums:"healthy,unhealthy,unknown,napping,starting"`
		Latency   int32              `json:"latency"`
		Timestamp int64              `json:"timestamp"`
	} // @name RouteStatus
	RouteStatuses  map[string][]Status // @name RouteStatuses
	RouteAggregate struct {
		Alias         string             `json:"alias"`
		DisplayName   string             `json:"display_name"`
		Uptime        float32            `json:"uptime"`
		Downtime      float32            `json:"downtime"`
		Idle          float32            `json:"idle"`
		AvgLatency    float32            `json:"avg_latency"`
		IsDocker      bool               `json:"is_docker"`
		CurrentStatus types.HealthStatus `json:"current_status" swaggertype:"string" enums:"healthy,unhealthy,unknown,napping,starting"`
		Statuses      []Status           `json:"statuses"`
	} // @name RouteUptimeAggregate
	Aggregated []RouteAggregate
)

var Poller = period.NewPoller("uptime", getStatuses, aggregateStatuses)

func getStatuses(ctx context.Context, _ *StatusByAlias) (*StatusByAlias, error) {
	return &StatusByAlias{
		Map:       routes.GetHealthInfo(),
		Timestamp: time.Now().Unix(),
	}, nil
}

func (s *Status) MarshalJSON() ([]byte, error) {
	return json.Marshal(map[string]any{
		"status":    s.Status.String(),
		"latency":   s.Latency,
		"timestamp": s.Timestamp,
	})
}

func aggregateStatuses(entries []*StatusByAlias, query url.Values) (int, Aggregated) {
	limit := metricsutils.QueryInt(query, "limit", 0)
	offset := metricsutils.QueryInt(query, "offset", 0)
	keyword := query.Get("keyword")

	statuses := make(RouteStatuses)
	for _, entry := range entries {
		for alias, status := range entry.Map {
			statuses[alias] = append(statuses[alias], Status{
				Status:    status.Status,
				Latency:   int32(status.Latency.Milliseconds()),
				Timestamp: entry.Timestamp,
			})
		}
	}
	if keyword != "" {
		for alias := range statuses {
			if !fuzzy.MatchFold(keyword, alias) {
				delete(statuses, alias)
			}
		}
	}
	return len(statuses), statuses.aggregate(limit, offset)
}

func (rs RouteStatuses) calculateInfo(statuses []Status) (up float32, down float32, idle float32, _ float32) {
	if len(statuses) == 0 {
		return 0, 0, 0, 0
	}
	total := float32(0)
	latency := float32(0)
	for _, status := range statuses {
		// ignoring unknown; treating napping and starting as downtime
		if status.Status == types.StatusUnknown {
			continue
		}
		switch {
		case status.Status == types.StatusHealthy:
			up++
		case status.Status.Idling():
			idle++
		default:
			down++
		}
		total++
		latency += float32(status.Latency)
	}
	if total == 0 {
		return 0, 0, 0, 0
	}
	return up / total, down / total, idle / total, latency / total
}

func (rs RouteStatuses) aggregate(limit int, offset int) Aggregated {
	n := len(rs)
	beg, end, ok := metricsutils.CalculateBeginEnd(n, limit, offset)
	if !ok {
		return Aggregated{}
	}
	i := 0
	sortedAliases := make([]string, n)
	for alias := range rs {
		sortedAliases[i] = alias
		i++
	}
	slices.Sort(sortedAliases)
	sortedAliases = sortedAliases[beg:end]
	result := make(Aggregated, len(sortedAliases))
	for i, alias := range sortedAliases {
		statuses := rs[alias]
		up, down, idle, latency := rs.calculateInfo(statuses)

		displayName := alias
		r, ok := routes.Get(alias)
		if !ok {
			// also search for excluded routes
			r = config.GetInstance().SearchRoute(alias)
		}
		if r != nil {
			displayName = r.DisplayName()
		}

		status := types.StatusUnknown
		if r != nil {
			mon := r.HealthMonitor()
			if mon != nil {
				status = mon.Status()
			}
		}

		result[i] = RouteAggregate{
			Alias:         alias,
			DisplayName:   displayName,
			Uptime:        up,
			Downtime:      down,
			Idle:          idle,
			AvgLatency:    latency,
			CurrentStatus: status,
			Statuses:      statuses,
			IsDocker:      r != nil && r.IsDocker(),
		}
	}
	return result
}

func (result Aggregated) MarshalJSON() ([]byte, error) {
	return json.Marshal([]RouteAggregate(result))
}
