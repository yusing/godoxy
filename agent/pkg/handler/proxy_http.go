package handler

import (
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/yusing/godoxy/agent/pkg/agent"
	"github.com/yusing/godoxy/agent/pkg/agentproxy"
	"github.com/yusing/goutils/http/reverseproxy"
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
	if cfg.ResponseHeaderTimeout > 0 {
		transport.ResponseHeaderTimeout = cfg.ResponseHeaderTimeout
	}
	if cfg.DisableCompression {
		transport.DisableCompression = true
	}

	// Strip the {API_BASE}/proxy/http prefix while preserving URL escaping.
	//
	// NOTE: `r.URL.Path` is decoded. If we rewrite it without keeping `RawPath`
	// in sync, Go may re-escape the path (e.g. turning "%5B" into "%255B"),
	// which breaks urls with percent-encoded characters, like Next.js static chunk URLs.
	prefix := agent.APIEndpointBase + agent.EndpointProxyHTTP
	r.URL.Path = strings.TrimPrefix(r.URL.Path, prefix)
	if r.URL.RawPath != "" {
		if after, ok := strings.CutPrefix(r.URL.RawPath, prefix); ok {
			r.URL.RawPath = after
		} else {
			// RawPath is no longer a valid encoding for Path; force Go to re-derive it.
			r.URL.RawPath = ""
		}
	}
	r.RequestURI = ""

	targetURL := *r.URL
	targetURL.Scheme = cfg.Scheme
	targetURL.Host = cfg.Host
	targetURL.Path = ""
	targetURL.RawPath = ""
	targetURL.RawQuery = ""

	transport.TLSClientConfig, err = cfg.BuildTLSConfig(&targetURL)
	if err != nil {
		http.Error(w, fmt.Sprintf("failed to build TLS client config: %s", err.Error()), http.StatusInternalServerError)
		return
	}

	rp := reverseproxy.NewReverseProxy(cfg.Host, &targetURL, transport)
	rp.ServeHTTP(w, r)
}
