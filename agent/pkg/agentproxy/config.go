package agentproxy

import (
	"encoding/base64"
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/yusing/godoxy/internal/types"
	strutils "github.com/yusing/goutils/strings"
)

type Config struct {
	Scheme string `json:"scheme,omitempty"`
	Host   string `json:"host,omitempty"` // host or host:port

	types.HTTPConfig
}

func ConfigFromHeaders(h http.Header) (Config, error) {
	_, hasScheme := h[HeaderXProxyScheme]
	_, hasConfig := h[HeaderXProxyConfig]
	if !hasScheme && !hasConfig {
		return proxyConfigFromHeadersLegacy(h), nil
	}
	cfg, err := proxyConfigFromHeaders(h)
	if err != nil {
		return cfg, err
	}
	if cfg.Host == "" {
		return cfg, errors.New("missing proxy host")
	}
	switch cfg.Scheme {
	case "http", "https", "h2c":
		return cfg, nil
	default:
		return cfg, errors.New("invalid proxy scheme")
	}
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

	return cfg
}

func proxyConfigFromHeaders(h http.Header) (cfg Config, err error) {
	cfg.Scheme = h.Get(HeaderXProxyScheme)
	cfg.Host = h.Get(HeaderXProxyHost)

	cfgBase64 := h.Get(HeaderXProxyConfig)
	cfgJSON, err := base64.StdEncoding.DecodeString(cfgBase64)
	if err != nil {
		return cfg, err
	}

	err = strutils.UnmarshalJSON(cfgJSON, &cfg.HTTPConfig)
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
	cfgJSON, _ := strutils.MarshalJSON(cfg.HTTPConfig)
	cfgBase64 := base64.StdEncoding.EncodeToString(cfgJSON)
	h.Set(HeaderXProxyConfig, cfgBase64)
}
