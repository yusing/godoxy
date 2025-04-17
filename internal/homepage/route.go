package homepage

import (
	"net/http"
	"net/url"

	"github.com/yusing/go-proxy/internal/utils/pool"
)

type route interface {
	pool.Object
	ProviderName() string
	Reference() string
	TargetURL() *url.URL
}

type httpRoute interface {
	route
	http.Handler
}
