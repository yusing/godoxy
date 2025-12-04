package handler

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strings"

	"github.com/bytedance/sonic"
	"github.com/yusing/godoxy/internal/types"
	"github.com/yusing/godoxy/internal/watcher/health/monitor"
)

func CheckHealth(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query()
	scheme := query.Get("scheme")
	if scheme == "" {
		http.Error(w, "missing scheme", http.StatusBadRequest)
		return
	}

	var (
		result types.HealthCheckResult
		err    error
	)
	switch scheme {
	case "fileserver":
		path := query.Get("path")
		if path == "" {
			http.Error(w, "missing path", http.StatusBadRequest)
			return
		}
		_, err := os.Stat(path)
		result = types.HealthCheckResult{Healthy: err == nil}
		if err != nil {
			result.Detail = err.Error()
		}
	case "http", "https": // path is optional
		host := query.Get("host")
		path := query.Get("path")
		if host == "" {
			http.Error(w, "missing host", http.StatusBadRequest)
			return
		}
		result, err = monitor.NewHTTPHealthMonitor(&url.URL{
			Scheme: scheme,
			Host:   host,
			Path:   path,
		}, healthCheckConfigFromRequest(r)).CheckHealth()
	case "tcp", "udp":
		host := query.Get("host")
		if host == "" {
			http.Error(w, "missing host", http.StatusBadRequest)
			return
		}
		hasPort := strings.Contains(host, ":")
		port := query.Get("port")
		if port != "" && hasPort {
			http.Error(w, "port and host with port cannot both be provided", http.StatusBadRequest)
			return
		}
		if port != "" {
			host = fmt.Sprintf("%s:%s", host, port)
		}
		result, err = monitor.NewRawHealthMonitor(&url.URL{
			Scheme: scheme,
			Host:   host,
		}, healthCheckConfigFromRequest(r)).CheckHealth()
	}

	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	sonic.ConfigDefault.NewEncoder(w).Encode(result)
}

func healthCheckConfigFromRequest(r *http.Request) types.HealthCheckConfig {
	// we only need timeout and base context because it's one shot request
	return types.HealthCheckConfig{
		Timeout: types.HealthCheckTimeoutDefault,
		BaseContext: func() context.Context {
			return r.Context()
		},
	}
}
