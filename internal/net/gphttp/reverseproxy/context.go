package reverseproxy

import (
	"context"
	"net/http"
)

var reverseProxyContextKey = struct{}{}

func (rp *ReverseProxy) WithContextValue(r *http.Request) *http.Request {
	return r.WithContext(context.WithValue(r.Context(), reverseProxyContextKey, rp))
}

func TryGetReverseProxy(r *http.Request) *ReverseProxy {
	if rp, ok := r.Context().Value(reverseProxyContextKey).(*ReverseProxy); ok {
		return rp
	}
	return nil
}

func TryGetUpstreamName(r *http.Request) string {
	if rp := TryGetReverseProxy(r); rp != nil {
		return rp.TargetName
	}
	return ""
}

func TryGetUpstreamScheme(r *http.Request) string {
	if rp := TryGetReverseProxy(r); rp != nil {
		return rp.TargetURL.Scheme
	}
	return ""
}

func TryGetUpstreamHost(r *http.Request) string {
	if rp := TryGetReverseProxy(r); rp != nil {
		return rp.TargetURL.Hostname()
	}
	return ""
}

func TryGetUpstreamPort(r *http.Request) string {
	if rp := TryGetReverseProxy(r); rp != nil {
		return rp.TargetURL.Port()
	}
	return ""
}

func TryGetUpstreamAddr(r *http.Request) string {
	if rp := TryGetReverseProxy(r); rp != nil {
		return rp.TargetURL.Host
	}
	return ""
}

func TryGetUpstreamURL(r *http.Request) string {
	if rp := TryGetReverseProxy(r); rp != nil {
		return rp.TargetURL.String()
	}
	return ""
}
