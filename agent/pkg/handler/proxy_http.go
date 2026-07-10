package handler

import (
	"container/list"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/yusing/godoxy/agent/pkg/agent"
	"github.com/yusing/godoxy/agent/pkg/agentproxy"
	"github.com/yusing/goutils/http/reverseproxy"
)

const maxCachedProxies = 64

type cachedProxy struct {
	key string
	rp  *reverseproxy.ReverseProxy
}

var proxyCache = struct {
	sync.Mutex
	entries map[string]*list.Element
	lru     list.List
}{entries: make(map[string]*list.Element)}

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

	rp, err := cachedReverseProxy(cfg, &targetURL)
	if err != nil {
		http.Error(w, fmt.Sprintf("failed to build proxy: %s", err.Error()), http.StatusInternalServerError)
		return
	}
	rp.ServeHTTP(w, r)
}

func cachedReverseProxy(cfg agentproxy.Config, targetURL *url.URL) (*reverseproxy.ReverseProxy, error) {
	keyBytes, err := sonic.Marshal(cfg)
	if err != nil {
		return nil, fmt.Errorf("marshal proxy config: %w", err)
	}
	key := string(keyBytes)

	proxyCache.Lock()
	defer proxyCache.Unlock()
	if elem, ok := proxyCache.entries[key]; ok {
		proxyCache.lru.MoveToFront(elem)
		return elem.Value.(cachedProxy).rp, nil
	}

	transport := NewTransport()
	if cfg.ResponseHeaderTimeout > 0 {
		transport.ResponseHeaderTimeout = cfg.ResponseHeaderTimeout
	}
	if cfg.DisableCompression {
		transport.DisableCompression = true
	}
	transport.TLSClientConfig, err = cfg.BuildTLSConfig(targetURL)
	if err != nil {
		return nil, err
	}

	rp := reverseproxy.NewReverseProxy(cfg.Host, targetURL, transport)
	elem := proxyCache.lru.PushFront(cachedProxy{key: key, rp: rp})
	proxyCache.entries[key] = elem
	if proxyCache.lru.Len() > maxCachedProxies {
		oldest := proxyCache.lru.Back()
		cached := oldest.Value.(cachedProxy)
		delete(proxyCache.entries, cached.key)
		proxyCache.lru.Remove(oldest)
		if transport, ok := cached.rp.Transport.(interface{ CloseIdleConnections() }); ok {
			transport.CloseIdleConnections()
		}
	}
	return rp, nil
}
