package gphttp

import (
	"crypto/tls"
	"net"
	"net/http"
	"time"
)

var DefaultDialer = net.Dialer{
	Timeout: 5 * time.Second,
}

func NewTransport() *http.Transport {
	return &http.Transport{
		Proxy:                 http.ProxyFromEnvironment,
		DialContext:           DefaultDialer.DialContext,
		ForceAttemptHTTP2:     true,
		MaxIdleConnsPerHost:   100,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
		// DisableCompression:    true, // Prevent double compression
		ResponseHeaderTimeout: 60 * time.Second,
		WriteBufferSize:       16 * 1024, // 16KB
		ReadBufferSize:        16 * 1024, // 16KB
	}
}

func NewTransportWithTLSConfig(tlsConfig *tls.Config) *http.Transport {
	tr := NewTransport()
	tr.TLSClientConfig = tlsConfig
	return tr
}
