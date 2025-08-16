package idlewatcher

import (
	"net/http"

	nettypes "github.com/yusing/go-proxy/internal/net/types"
	"github.com/yusing/go-proxy/internal/types"
)

type Waker interface {
	types.HealthMonitor
	http.Handler
	nettypes.Stream
	Wake() error
}
