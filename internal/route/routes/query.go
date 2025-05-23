package routes

import (
	"encoding/json"
	"time"

	"github.com/yusing/go-proxy/internal/homepage"
	"github.com/yusing/go-proxy/internal/watcher/health"
)

func getHealthInfo(r Route) map[string]string {
	mon := r.HealthMonitor()
	if mon == nil {
		return map[string]string{
			"status":  "unknown",
			"uptime":  "n/a",
			"latency": "n/a",
			"detail":  "n/a",
		}
	}
	return map[string]string{
		"status":  mon.Status().String(),
		"uptime":  mon.Uptime().Round(time.Second).String(),
		"latency": mon.Latency().Round(time.Microsecond).String(),
		"detail":  mon.Detail(),
	}
}

type HealthInfoRaw struct {
	Status  health.Status `json:"status"`
	Latency time.Duration `json:"latency"`
}

func (info *HealthInfoRaw) MarshalJSON() ([]byte, error) {
	return json.Marshal(map[string]any{
		"status":  info.Status.String(),
		"latency": info.Latency.Milliseconds(),
	})
}

func (info *HealthInfoRaw) UnmarshalJSON(data []byte) error {
	var v map[string]any
	if err := json.Unmarshal(data, &v); err != nil {
		return err
	}
	if status, ok := v["status"].(string); ok {
		info.Status = health.NewStatus(status)
	}
	if latency, ok := v["latency"].(float64); ok {
		info.Latency = time.Duration(latency)
	}
	return nil
}

func getHealthInfoRaw(r Route) *HealthInfoRaw {
	mon := r.HealthMonitor()
	if mon == nil {
		return &HealthInfoRaw{
			Status:  health.StatusUnknown,
			Latency: time.Duration(0),
		}
	}
	return &HealthInfoRaw{
		Status:  mon.Status(),
		Latency: mon.Latency(),
	}
}

func HealthMap() map[string]map[string]string {
	healthMap := make(map[string]map[string]string, NumRoutes())
	for r := range Iter {
		healthMap[r.Name()] = getHealthInfo(r)
	}
	return healthMap
}

func HealthInfo() map[string]*HealthInfoRaw {
	healthMap := make(map[string]*HealthInfoRaw, NumRoutes())
	for r := range Iter {
		healthMap[r.Name()] = getHealthInfoRaw(r)
	}
	return healthMap
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

func HomepageConfig(categoryFilter, providerFilter string) homepage.Homepage {
	hp := make(homepage.Homepage)

	for _, r := range HTTP.Iter {
		if providerFilter != "" && r.ProviderName() != providerFilter {
			continue
		}
		item := r.HomepageItem()
		if categoryFilter != "" && item.Category != categoryFilter {
			continue
		}
		hp.Add(item)
	}
	return hp
}

func ByProvider() map[string][]Route {
	rts := make(map[string][]Route)
	for r := range Iter {
		rts[r.ProviderName()] = append(rts[r.ProviderName()], r)
	}
	return rts
}
