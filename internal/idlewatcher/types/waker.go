package idlewatcher

import (
	"net/http"

	net "github.com/yusing/go-proxy/internal/net/types"
	"github.com/yusing/go-proxy/internal/watcher/health"
)

type Waker interface {
	health.HealthMonitor
	http.Handler
	net.Stream
	Wake() error
}
