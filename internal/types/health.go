package types

import (
	"encoding/json"
	"fmt"
	"net/url"
	"strconv"
	"time"

	"github.com/bytedance/sonic"
	"github.com/yusing/goutils/task"
)

type (
	HealthStatus       uint8  // @name	HealthStatus
	HealthStatusString string // @name	HealthStatusString

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
		Name     string             `json:"name"`
		Config   *HealthCheckConfig `json:"config"`
		Started  int64              `json:"started"` // unix timestamp in seconds
		Status   HealthStatusString `json:"status"`
		Uptime   float64            `json:"uptime"`   // uptime in seconds
		Latency  int64              `json:"latency"`  // latency in milliseconds
		LastSeen int64              `json:"lastSeen"` // unix timestamp in seconds
		Detail   string             `json:"detail"`
		URL      string             `json:"url"`
		Extra    *HealthExtra       `json:"extra,omitempty" extensions:"x-nullable"`
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

	HealthInfoWithoutDetail struct {
		Status  HealthStatus  `json:"status" swaggertype:"string" enums:"healthy,unhealthy,napping,starting,error,unknown"`
		Uptime  time.Duration `json:"uptime" swaggertype:"number"`  // uptime in milliseconds
		Latency time.Duration `json:"latency" swaggertype:"number"` // latency in microseconds
	} // @name HealthInfoWithoutDetail

	HealthInfo struct {
		HealthInfoWithoutDetail
		Detail string `json:"detail"`
	} // @name HealthInfo

	HealthMap = map[string]HealthStatusString // @name	HealthMap
)

const (
	StatusUnknown HealthStatus = 0
	StatusHealthy HealthStatus = (1 << iota)
	StatusNapping
	StatusStarting
	StatusUnhealthy
	StatusError

	StatusUnknownStr   HealthStatusString = "unknown"
	StatusHealthyStr   HealthStatusString = "healthy"
	StatusNappingStr   HealthStatusString = "napping"
	StatusStartingStr  HealthStatusString = "starting"
	StatusUnhealthyStr HealthStatusString = "unhealthy"
	StatusErrorStr     HealthStatusString = "error"

	NumStatuses int = iota - 1

	HealthyMask = StatusHealthy | StatusNapping | StatusStarting
	IdlingMask  = StatusNapping | StatusStarting
)

var (
	StatusHealthyStr2   HealthStatusString = HealthStatusString(strconv.Itoa(int(StatusHealthy)))
	StatusNappingStr2   HealthStatusString = HealthStatusString(strconv.Itoa(int(StatusNapping)))
	StatusStartingStr2  HealthStatusString = HealthStatusString(strconv.Itoa(int(StatusStarting)))
	StatusUnhealthyStr2 HealthStatusString = HealthStatusString(strconv.Itoa(int(StatusUnhealthy)))
	StatusErrorStr2     HealthStatusString = HealthStatusString(strconv.Itoa(int(StatusError)))
)

func NewHealthStatusFromString(s string) HealthStatus {
	switch HealthStatusString(s) {
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

func (s HealthStatus) StatusString() HealthStatusString {
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

// String implements fmt.Stringer.
func (s HealthStatus) String() string {
	return string(s.StatusString())
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
		Name:     jsonRepr.Name,
		Config:   jsonRepr.Config,
		Started:  jsonRepr.Started.Unix(),
		Status:   HealthStatusString(jsonRepr.Status.String()),
		Uptime:   jsonRepr.Uptime.Seconds(),
		Latency:  jsonRepr.Latency.Milliseconds(),
		LastSeen: jsonRepr.LastSeen.Unix(),
		Detail:   jsonRepr.Detail,
		URL:      url,
		Extra:    jsonRepr.Extra,
	})
}
