package homepage

import (
	"net/http"

	nettypes "github.com/yusing/go-proxy/internal/net/types"
	"github.com/yusing/go-proxy/internal/utils/pool"
)

type route interface {
	pool.Object
	ProviderName() string
	References() []string
	TargetURL() *nettypes.URL
}

type httpRoute interface {
	route
	http.Handler
}
