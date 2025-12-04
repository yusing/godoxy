package homepage

import (
	"net/http"

	nettypes "github.com/yusing/godoxy/internal/net/types"
	"github.com/yusing/goutils/pool"
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
