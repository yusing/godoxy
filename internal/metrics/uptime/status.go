package uptime

import (
	"fmt"

	"github.com/yusing/go-proxy/internal/watcher/health"
)

type Status struct {
	Status    health.Status
	Latency   int64
	Timestamp int64
}

type RouteStatuses map[string][]*Status

func (s *Status) MarshalJSONTo(buf []byte) []byte {
	return fmt.Appendf(buf,
		`{"status":"%s","latency":"%d","timestamp":"%d"}`,
		s.Status, s.Latency, s.Timestamp,
	)
}
