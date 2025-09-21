package handler

import (
	"fmt"
	"net/http"
	"net/http/httputil"
	"time"

	"github.com/yusing/go-proxy/agent/pkg/agent"
	"github.com/yusing/go-proxy/agent/pkg/agentproxy"
)

func NewTransport() *http.Transport {
	return &http.Transport{
		MaxIdleConnsPerHost:   100,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
		ResponseHeaderTimeout: 60 * time.Second,
		WriteBufferSize:       16 * 1024, // 16KB
		ReadBufferSize:        16 * 1024, // 16KB
	}
}

func ProxyHTTP(w http.ResponseWriter, r *http.Request) {
	cfg, err := agentproxy.ConfigFromHeaders(r.Header)
	if err != nil {
		http.Error(w, fmt.Sprintf("failed to parse agent proxy config: %s", err.Error()), http.StatusBadRequest)
		return
	}

	transport := NewTransport()
	transport.TLSClientConfig, err = cfg.BuildTLSConfig(r.URL)
	if err != nil {
		http.Error(w, fmt.Sprintf("failed to build TLS client config: %s", err.Error()), http.StatusInternalServerError)
		return
	}

	r.URL.Scheme = ""
	r.URL.Host = ""
	r.URL.Path = r.URL.Path[agent.HTTPProxyURLPrefixLen:] // strip the {API_BASE}/proxy/http prefix
	r.RequestURI = r.URL.String()

	rp := &httputil.ReverseProxy{
		Director: func(r *http.Request) {
			r.URL.Scheme = cfg.Scheme
			r.URL.Host = cfg.Host
		},
		Transport: transport,
	}
	rp.ServeHTTP(w, r)
}
