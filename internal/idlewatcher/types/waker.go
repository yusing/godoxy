package idlewatcher

import (
	"net/http"

	nettypes "github.com/yusing/go-proxy/internal/net/types"
	"github.com/yusing/go-proxy/internal/watcher/health"
)

type Waker interface {
	health.HealthMonitor
	http.Handler
	nettypes.Stream
	Wake() error
}
