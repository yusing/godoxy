package homepage

import (
	"net/http"

	net "github.com/yusing/go-proxy/internal/net/types"
)

type route interface {
	TargetName() string
	ProviderName() string
	Reference() string
	TargetURL() *net.URL
}

type httpRoute interface {
	route
	http.Handler
}
