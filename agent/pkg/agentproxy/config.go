package agentproxy

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	route "github.com/yusing/go-proxy/internal/route/types"
)

type Config struct {
	Scheme string `json:"scheme,omitempty"`
	Host   string `json:"host,omitempty"` // host or host:port

	route.HTTPConfig
}

func ConfigFromHeaders(h http.Header) (Config, error) {
	cfg, err := proxyConfigFromHeaders(h)
	if err != nil {
		return cfg, err
	}
	if cfg.Host == "" {
		cfg = proxyConfigFromHeadersLegacy(h)
	}
	return cfg, nil
}

func proxyConfigFromHeadersLegacy(h http.Header) (cfg Config) {
	cfg.Host = h.Get(HeaderXProxyHost)
	isHTTPS, _ := strconv.ParseBool(h.Get(HeaderXProxyHTTPS))
	cfg.NoTLSVerify, _ = strconv.ParseBool(h.Get(HeaderXProxySkipTLSVerify))
	responseHeaderTimeout, err := strconv.Atoi(h.Get(HeaderXProxyResponseHeaderTimeout))
	if err != nil {
		responseHeaderTimeout = 0
	}
	cfg.ResponseHeaderTimeout = time.Duration(responseHeaderTimeout) * time.Second

	cfg.Scheme = "http"
	if isHTTPS {
		cfg.Scheme = "https"
	}

	return
}

func proxyConfigFromHeaders(h http.Header) (cfg Config, err error) {
	cfg.Scheme = h.Get(HeaderXProxyScheme)
	cfg.Host = h.Get(HeaderXProxyHost)

	cfgBase64 := h.Get(HeaderXProxyConfig)
	cfgJSON, err := base64.StdEncoding.DecodeString(cfgBase64)
	if err != nil {
		return cfg, err
	}

	err = json.Unmarshal(cfgJSON, &cfg)
	return cfg, err
}

func (cfg *Config) SetAgentProxyConfigHeadersLegacy(h http.Header) {
	h.Set(HeaderXProxyHost, cfg.Host)
	h.Set(HeaderXProxyHTTPS, strconv.FormatBool(cfg.Scheme == "https"))
	h.Set(HeaderXProxySkipTLSVerify, strconv.FormatBool(cfg.NoTLSVerify))
	h.Set(HeaderXProxyResponseHeaderTimeout, strconv.Itoa(int(cfg.ResponseHeaderTimeout.Round(time.Second).Seconds())))
}

func (cfg *Config) SetAgentProxyConfigHeaders(h http.Header) {
	h.Set(HeaderXProxyHost, cfg.Host)
	h.Set(HeaderXProxyScheme, string(cfg.Scheme))
	cfgJSON, _ := json.Marshal(cfg.HTTPConfig)
	cfgBase64 := base64.StdEncoding.EncodeToString(cfgJSON)
	h.Set(HeaderXProxyConfig, cfgBase64)
}
