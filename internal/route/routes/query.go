package routes

import (
	"encoding/json"
	"fmt"
	"math"
	"net/url"
	"strings"
	"time"

	"github.com/yusing/go-proxy/internal/homepage"
	"github.com/yusing/go-proxy/internal/types"
)

type HealthInfo struct {
	Status  types.HealthStatus `json:"status" swaggertype:"string" enums:"healthy,unhealthy,napping,starting,error,unknown"`
	Uptime  time.Duration      `json:"uptime" swaggertype:"number"`  // uptime in milliseconds
	Latency time.Duration      `json:"latency" swaggertype:"number"` // latency in microseconds
	Detail  string             `json:"detail"`
}

func (info *HealthInfo) MarshalJSON() ([]byte, error) {
	return json.Marshal(map[string]any{
		"status":  info.Status.String(),
		"latency": info.Latency.Microseconds(),
		"uptime":  info.Uptime.Milliseconds(),
		"detail":  info.Detail,
	})
}

func (info *HealthInfo) UnmarshalJSON(data []byte) error {
	var v struct {
		Status  string `json:"status"`
		Latency int64  `json:"latency"`
		Uptime  int64  `json:"uptime"`
		Detail  string `json:"detail"`
	}
	if err := json.Unmarshal(data, &v); err != nil {
		return err
	}

	// overflow check
	if math.MaxInt64/time.Microsecond < time.Duration(v.Latency) {
		return fmt.Errorf("latency overflow: %d", v.Latency)
	}
	if math.MaxInt64/time.Millisecond < time.Duration(v.Uptime) {
		return fmt.Errorf("uptime overflow: %d", v.Uptime)
	}

	info.Status = types.NewHealthStatusFromString(v.Status)
	info.Latency = time.Duration(v.Latency) * time.Microsecond
	info.Uptime = time.Duration(v.Uptime) * time.Millisecond
	info.Detail = v.Detail
	return nil
}

func GetHealthInfo() map[string]*HealthInfo {
	healthMap := make(map[string]*HealthInfo, NumRoutes())
	for r := range Iter {
		healthMap[r.Name()] = getHealthInfo(r)
	}
	return healthMap
}

func getHealthInfo(r types.Route) *HealthInfo {
	mon := r.HealthMonitor()
	if mon == nil {
		return &HealthInfo{
			Status: types.StatusUnknown,
			Detail: "n/a",
		}
	}
	return &HealthInfo{
		Status:  mon.Status(),
		Uptime:  mon.Uptime(),
		Latency: mon.Latency(),
		Detail:  mon.Detail(),
	}
}

func HomepageCategories() []string {
	check := make(map[string]struct{})
	categories := make([]string, 0)
	for _, r := range HTTP.Iter {
		item := r.HomepageConfig()
		if item == nil || item.Category == "" {
			continue
		}
		if _, ok := check[item.Category]; ok {
			continue
		}
		check[item.Category] = struct{}{}
		categories = append(categories, item.Category)
	}
	return categories
}

func HomepageItems(proto, hostname, categoryFilter, providerFilter string) homepage.Homepage {
	switch proto {
	case "http", "https":
	default:
		proto = "http"
	}

	hp := make(homepage.Homepage)

	for _, r := range HTTP.Iter {
		if providerFilter != "" && r.ProviderName() != providerFilter {
			continue
		}
		item := r.HomepageItem()
		if categoryFilter != "" && item.Category != categoryFilter {
			continue
		}

		// clear url if invalid
		_, err := url.Parse(item.URL)
		if err != nil {
			item.URL = ""
		}

		// append hostname if provided and only if alias is not FQDN
		if hostname != "" && item.URL == "" {
			if !strings.Contains(item.Alias, ".") {
				item.URL = fmt.Sprintf("%s://%s.%s", proto, item.Alias, hostname)
			}
		}

		// prepend protocol if not exists
		if !strings.HasPrefix(item.URL, "http://") && !strings.HasPrefix(item.URL, "https://") {
			item.URL = fmt.Sprintf("%s://%s", proto, item.URL)
		}

		hp.Add(item)
	}
	return hp
}

func ByProvider() map[string][]types.Route {
	rts := make(map[string][]types.Route)
	for r := range Iter {
		rts[r.ProviderName()] = append(rts[r.ProviderName()], r)
	}
	return rts
}
