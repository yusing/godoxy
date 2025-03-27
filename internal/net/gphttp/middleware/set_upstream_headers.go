package middleware

import (
	"net/http"

	"github.com/yusing/go-proxy/internal/net/gphttp/httpheaders"
	"github.com/yusing/go-proxy/internal/net/gphttp/reverseproxy"
)

// internal use only.
type setUpstreamHeaders struct {
	Name, Scheme, Host, Port string
}

var suh = NewMiddleware[setUpstreamHeaders]()

func newSetUpstreamHeaders(rp *reverseproxy.ReverseProxy) *Middleware {
	m, err := suh.New(OptionsRaw{
		"name":   rp.TargetName,
		"scheme": rp.TargetURL.Scheme,
		"host":   rp.TargetURL.Hostname(),
		"port":   rp.TargetURL.Port(),
	})
	if err != nil {
		panic(err)
	}
	return m
}

// before implements RequestModifier.
func (s setUpstreamHeaders) before(w http.ResponseWriter, r *http.Request) (proceed bool) {
	r.Header.Set(httpheaders.HeaderUpstreamName, s.Name)
	r.Header.Set(httpheaders.HeaderUpstreamScheme, s.Scheme)
	r.Header.Set(httpheaders.HeaderUpstreamHost, s.Host)
	r.Header.Set(httpheaders.HeaderUpstreamPort, s.Port)
	return true
}
