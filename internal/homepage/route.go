package homepage

import (
	"net/http"
	"net/url"
)

type route interface {
	TargetName() string
	ProviderName() string
	Reference() string
	TargetURL() *url.URL
}

type httpRoute interface {
	route
	http.Handler
}
