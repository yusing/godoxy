package handler

import (
	"crypto/tls"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"github.com/yusing/go-proxy/agent/pkg/agent"
	"github.com/yusing/go-proxy/agent/pkg/agentproxy"
	"github.com/yusing/go-proxy/internal/logging"
	"github.com/yusing/go-proxy/internal/net/gphttp"
	"github.com/yusing/go-proxy/internal/net/gphttp/reverseproxy"
	"github.com/yusing/go-proxy/internal/net/types"
	"github.com/yusing/go-proxy/internal/utils/strutils"
)

func ProxyHTTP(w http.ResponseWriter, r *http.Request) {
	host := r.Header.Get(agentproxy.HeaderXProxyHost)
	isHTTPS := strutils.ParseBool(r.Header.Get(agentproxy.HeaderXProxyHTTPS))
	skipTLSVerify := strutils.ParseBool(r.Header.Get(agentproxy.HeaderXProxySkipTLSVerify))
	responseHeaderTimeout, err := strconv.Atoi(r.Header.Get(agentproxy.HeaderXProxyResponseHeaderTimeout))
	if err != nil {
		responseHeaderTimeout = 0
	}

	if host == "" {
		http.Error(w, "missing required headers", http.StatusBadRequest)
		return
	}

	scheme := "http"
	if isHTTPS {
		scheme = "https"
	}

	var transport *http.Transport
	if skipTLSVerify {
		transport = gphttp.NewTransportWithTLSConfig(&tls.Config{InsecureSkipVerify: true})
	} else {
		transport = gphttp.NewTransport()
	}

	if responseHeaderTimeout > 0 {
		transport.ResponseHeaderTimeout = time.Duration(responseHeaderTimeout) * time.Second
	}

	r.URL.Scheme = ""
	r.URL.Host = ""
	r.URL.Path = r.URL.Path[agent.HTTPProxyURLPrefixLen:] // strip the {API_BASE}/proxy/http prefix
	r.RequestURI = r.URL.String()
	r.URL.Host = host
	r.URL.Scheme = scheme

	logging.Debug().Msgf("proxy http request: %s %s", r.Method, r.URL.String())

	rp := reverseproxy.NewReverseProxy("agent", types.NewURL(&url.URL{
		Scheme: scheme,
		Host:   host,
	}), transport)
	rp.ServeHTTP(w, r)
}
