package handler

import (
	"crypto/tls"
	"net/http"
	"net/http/httputil"
	"strconv"
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
	host := r.Header.Get(agentproxy.HeaderXProxyHost)
	isHTTPS, _ := strconv.ParseBool(r.Header.Get(agentproxy.HeaderXProxyHTTPS))
	skipTLSVerify, _ := strconv.ParseBool(r.Header.Get(agentproxy.HeaderXProxySkipTLSVerify))
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

	transport := NewTransport()
	if skipTLSVerify {
		transport.TLSClientConfig = &tls.Config{InsecureSkipVerify: true}
	}

	if responseHeaderTimeout > 0 {
		transport.ResponseHeaderTimeout = time.Duration(responseHeaderTimeout) * time.Second
	}

	r.URL.Scheme = ""
	r.URL.Host = ""
	r.URL.Path = r.URL.Path[agent.HTTPProxyURLPrefixLen:] // strip the {API_BASE}/proxy/http prefix
	r.RequestURI = r.URL.String()

	rp := &httputil.ReverseProxy{
		Director: func(r *http.Request) {
			r.URL.Scheme = scheme
			r.URL.Host = host
		},
		Transport: transport,
	}
	rp.ServeHTTP(w, r)
}
