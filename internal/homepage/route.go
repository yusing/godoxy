package homepage

import (
	"net/http"

	gpnet "github.com/yusing/go-proxy/internal/net/types"
	"github.com/yusing/go-proxy/internal/utils/pool"
)

type route interface {
	pool.Object
	ProviderName() string
	Reference() string
	TargetURL() *gpnet.URL
}

type httpRoute interface {
	route
	http.Handler
}
