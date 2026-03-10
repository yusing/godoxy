package handler

import (
	"encoding/json"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	healthcheck "github.com/yusing/godoxy/internal/health/check"
	"github.com/yusing/godoxy/internal/types"
)

func CheckHealth(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query()
	scheme := query.Get("scheme")
	if scheme == "" {
		http.Error(w, "missing scheme", http.StatusBadRequest)
		return
	}
	timeout := parseMsOrDefault(query.Get("timeout"))

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
		result, err = healthcheck.FileServer(path)
	case "http", "https", "h2c": // path is optional
		host := query.Get("host")
		path := query.Get("path")
		if host == "" {
			http.Error(w, "missing host", http.StatusBadRequest)
			return
		}
		url := url.URL{Scheme: scheme, Host: host}
		if scheme == "h2c" {
			result, err = healthcheck.H2C(r.Context(), &url, http.MethodHead, path, timeout)
		} else {
			result, err = healthcheck.HTTP(&url, http.MethodHead, path, timeout)
		}
	case "tcp", "udp", "tcp4", "udp4", "tcp6", "udp6":
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
			host = net.JoinHostPort(host, port)
		}
		url := url.URL{Scheme: scheme, Host: host}
		result, err = healthcheck.Stream(r.Context(), &url, timeout)
	}

	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(result)
}

func parseMsOrDefault(msStr string) time.Duration {
	if msStr == "" {
		return types.HealthCheckTimeoutDefault
	}

	timeoutMs, _ := strconv.ParseInt(msStr, 10, 64)
	if timeoutMs == 0 {
		return types.HealthCheckTimeoutDefault
	}

	return time.Duration(timeoutMs) * time.Millisecond
}
