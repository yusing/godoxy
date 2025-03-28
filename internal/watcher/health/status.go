package health

import "encoding/json"

type Status uint8

const (
	StatusUnknown Status = 0
	StatusHealthy        = (1 << iota)
	StatusNapping
	StatusStarting
	StatusUnhealthy
	StatusError

	NumStatuses int = iota - 1

	HealthyMask = StatusHealthy | StatusNapping | StatusStarting
	IdlingMask  = StatusNapping | StatusStarting
)

func (s Status) String() string {
	switch s {
	case StatusHealthy:
		return "healthy"
	case StatusUnhealthy:
		return "unhealthy"
	case StatusNapping:
		return "napping"
	case StatusStarting:
		return "starting"
	case StatusError:
		return "error"
	default:
		return "unknown"
	}
}

func (s Status) MarshalJSON() ([]byte, error) {
	return []byte(`"` + s.String() + `"`), nil
}

func (s *Status) UnmarshalJSON(data []byte) error {
	var str string
	if err := json.Unmarshal(data, &str); err != nil {
		return err
	}
	switch str {
	case "healthy":
		*s = StatusHealthy
	case "unhealthy":
		*s = StatusUnhealthy
	case "napping":
		*s = StatusNapping
	case "starting":
		*s = StatusStarting
	case "error":
		*s = StatusError
	default:
		*s = StatusUnknown
	}
	return nil
}

func (s Status) Good() bool {
	return s&HealthyMask != 0
}

func (s Status) Bad() bool {
	return s&HealthyMask == 0
}

func (s Status) Idling() bool {
	return s&IdlingMask != 0
}
