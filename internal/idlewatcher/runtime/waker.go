package runtime

import (
	"context"
	"net/http"

	"github.com/yusing/godoxy/internal/health"
	nettypes "github.com/yusing/godoxy/internal/net/types"
)

type Waker interface {
	health.HealthMonitor
	http.Handler
	nettypes.Stream
	Wake(ctx context.Context) error
}
