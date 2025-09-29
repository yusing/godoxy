package types

import (
	"encoding/json"
	"fmt"
	"net/url"
	"strconv"
	"time"

	"github.com/bytedance/sonic"
	strutils "github.com/yusing/goutils/strings"
	"github.com/yusing/goutils/task"
)

type (
	HealthStatus uint8

	HealthCheckResult struct {
		Healthy bool          `json:"healthy"`
		Detail  string        `json:"detail"`
		Latency time.Duration `json:"latency"`
	} //	@name	HealthCheckResult
	WithHealthInfo interface {
		Status() HealthStatus
		Uptime() time.Duration
		Latency() time.Duration
		Detail() string
	}
	HealthMonitor interface {
		task.TaskStarter
		task.TaskFinisher
		fmt.Stringer
		WithHealthInfo
		Name() string
		json.Marshaler
	}
	HealthChecker interface {
		CheckHealth() (result HealthCheckResult, err error)
		URL() *url.URL
		Config() *HealthCheckConfig
		UpdateURL(url *url.URL)
	}
	HealthMonCheck interface {
		HealthMonitor
		HealthChecker
	}
	HealthJSON struct {
		Name        string             `json:"name"`
		Config      *HealthCheckConfig `json:"config"`
		Started     int64              `json:"started"`
		StartedStr  string             `json:"startedStr"`
		Status      string             `json:"status"`
		Uptime      float64            `json:"uptime"`
		UptimeStr   string             `json:"uptimeStr"`
		Latency     float64            `json:"latency"`
		LatencyStr  string             `json:"latencyStr"`
		LastSeen    int64              `json:"lastSeen"`
		LastSeenStr string             `json:"lastSeenStr"`
		Detail      string             `json:"detail"`
		URL         string             `json:"url"`
		Extra       *HealthExtra       `json:"extra" extensions:"x-nullable"`
	} // @name HealthJSON

	HealthJSONRepr struct {
		Name     string
		Config   *HealthCheckConfig
		Status   HealthStatus
		Started  time.Time
		Uptime   time.Duration
		Latency  time.Duration
		LastSeen time.Time
		Detail   string
		URL      *url.URL
		Extra    *HealthExtra
	}

	HealthExtra struct {
		Config *LoadBalancerConfig `json:"config"`
		Pool   map[string]any      `json:"pool"`
	} // @name HealthExtra
)

const (
	StatusUnknown HealthStatus = 0
	StatusHealthy HealthStatus = (1 << iota)
	StatusNapping
	StatusStarting
	StatusUnhealthy
	StatusError

	StatusUnknownStr   = "unknown"
	StatusHealthyStr   = "healthy"
	StatusNappingStr   = "napping"
	StatusStartingStr  = "starting"
	StatusUnhealthyStr = "unhealthy"
	StatusErrorStr     = "error"

	NumStatuses int = iota - 1

	HealthyMask = StatusHealthy | StatusNapping | StatusStarting
	IdlingMask  = StatusNapping | StatusStarting
)

var (
	StatusHealthyStr2   = strconv.Itoa(int(StatusHealthy))
	StatusNappingStr2   = strconv.Itoa(int(StatusNapping))
	StatusStartingStr2  = strconv.Itoa(int(StatusStarting))
	StatusUnhealthyStr2 = strconv.Itoa(int(StatusUnhealthy))
	StatusErrorStr2     = strconv.Itoa(int(StatusError))
)

func NewHealthStatusFromString(s string) HealthStatus {
	switch s {
	case StatusHealthyStr, StatusHealthyStr2:
		return StatusHealthy
	case StatusUnhealthyStr, StatusUnhealthyStr2:
		return StatusUnhealthy
	case StatusNappingStr, StatusNappingStr2:
		return StatusNapping
	case StatusStartingStr, StatusStartingStr2:
		return StatusStarting
	case StatusErrorStr, StatusErrorStr2:
		return StatusError
	default:
		return StatusUnknown
	}
}

func (s HealthStatus) String() string {
	switch s {
	case StatusHealthy:
		return StatusHealthyStr
	case StatusUnhealthy:
		return StatusUnhealthyStr
	case StatusNapping:
		return StatusNappingStr
	case StatusStarting:
		return StatusStartingStr
	case StatusError:
		return StatusErrorStr
	default:
		return StatusUnknownStr
	}
}

func (s HealthStatus) Good() bool {
	return s&HealthyMask != 0
}

func (s HealthStatus) Bad() bool {
	return s&HealthyMask == 0
}

func (s HealthStatus) Idling() bool {
	return s&IdlingMask != 0
}

func (s HealthStatus) MarshalJSON() ([]byte, error) {
	return strconv.AppendQuote(nil, s.String()), nil
}

func (s *HealthStatus) UnmarshalJSON(data []byte) error {
	var v string
	if err := sonic.Unmarshal(data, &v); err != nil {
		return fmt.Errorf("failed to unmarshal health status: %w", err)
	}

	*s = NewHealthStatusFromString(v)
	return nil
}

func (jsonRepr *HealthJSONRepr) MarshalJSON() ([]byte, error) {
	var url string
	if jsonRepr.URL != nil {
		url = jsonRepr.URL.String()
	}
	if url == "http://:0" {
		url = ""
	}
	return sonic.Marshal(HealthJSON{
		Name:        jsonRepr.Name,
		Config:      jsonRepr.Config,
		Started:     jsonRepr.Started.Unix(),
		StartedStr:  strutils.FormatTime(jsonRepr.Started),
		Status:      jsonRepr.Status.String(),
		Uptime:      jsonRepr.Uptime.Seconds(),
		UptimeStr:   strutils.FormatDuration(jsonRepr.Uptime),
		Latency:     jsonRepr.Latency.Seconds(),
		LatencyStr:  strconv.Itoa(int(jsonRepr.Latency.Milliseconds())) + " ms",
		LastSeen:    jsonRepr.LastSeen.Unix(),
		LastSeenStr: strutils.FormatLastSeen(jsonRepr.LastSeen),
		Detail:      jsonRepr.Detail,
		URL:         url,
		Extra:       jsonRepr.Extra,
	})
}
