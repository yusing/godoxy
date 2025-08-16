package uptime

import (
	"context"
	"encoding/json"
	"net/url"
	"strings"
	"time"

	"slices"

	"github.com/lithammer/fuzzysearch/fuzzy"
	"github.com/yusing/go-proxy/internal/metrics/period"
	metricsutils "github.com/yusing/go-proxy/internal/metrics/utils"
	"github.com/yusing/go-proxy/internal/route/routes"
	"github.com/yusing/go-proxy/internal/types"
)

type (
	StatusByAlias struct {
		Map       map[string]*routes.HealthInfo `json:"statuses"`
		Timestamp int64                         `json:"timestamp"`
	} // @name RouteStatusesByAlias
	Status struct {
		Status    types.HealthStatus `json:"status" swaggertype:"string" enums:"healthy,unhealthy,unknown,napping,starting"`
		Latency   int64              `json:"latency"`
		Timestamp int64              `json:"timestamp"`
	} // @name RouteStatus
	RouteStatuses  map[string][]*Status // @name RouteStatuses
	RouteAggregate struct {
		Alias       string    `json:"alias"`
		DisplayName string    `json:"display_name"`
		Uptime      float64   `json:"uptime"`
		Downtime    float64   `json:"downtime"`
		Idle        float64   `json:"idle"`
		AvgLatency  float64   `json:"avg_latency"`
		Statuses    []*Status `json:"statuses"`
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
			statuses[alias] = append(statuses[alias], &Status{
				Status:    status.Status,
				Latency:   status.Latency.Milliseconds(),
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

func (rs RouteStatuses) calculateInfo(statuses []*Status) (up float64, down float64, idle float64, _ float64) {
	if len(statuses) == 0 {
		return 0, 0, 0, 0
	}
	total := float64(0)
	latency := float64(0)
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
		latency += float64(status.Latency)
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
	// unknown statuses are at the end, then sort by alias
	slices.SortFunc(sortedAliases, func(a, b string) int {
		if rs[a][len(rs[a])-1].Status == types.StatusUnknown {
			return 1
		}
		if rs[b][len(rs[b])-1].Status == types.StatusUnknown {
			return -1
		}
		return strings.Compare(a, b)
	})
	sortedAliases = sortedAliases[beg:end]
	result := make(Aggregated, len(sortedAliases))
	for i, alias := range sortedAliases {
		statuses := rs[alias]
		up, down, idle, latency := rs.calculateInfo(statuses)
		result[i] = RouteAggregate{
			Alias:      alias,
			Uptime:     up,
			Downtime:   down,
			Idle:       idle,
			AvgLatency: latency,
			Statuses:   statuses,
		}
		r, ok := routes.Get(alias)
		if ok {
			result[i].DisplayName = r.HomepageConfig().Name
		} else {
			result[i].DisplayName = alias
		}
	}
	return result
}

func (result Aggregated) MarshalJSON() ([]byte, error) {
	return json.Marshal([]RouteAggregate(result))
}
