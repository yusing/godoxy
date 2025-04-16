package api

import (
	"net/http"

	"github.com/prometheus/client_golang/prometheus/promhttp"
	v1 "github.com/yusing/go-proxy/internal/api/v1"
	"github.com/yusing/go-proxy/internal/api/v1/auth"
	"github.com/yusing/go-proxy/internal/api/v1/certapi"
	"github.com/yusing/go-proxy/internal/api/v1/dockerapi"
	"github.com/yusing/go-proxy/internal/api/v1/favicon"
	"github.com/yusing/go-proxy/internal/common"
	config "github.com/yusing/go-proxy/internal/config/types"
	"github.com/yusing/go-proxy/internal/logging"
	"github.com/yusing/go-proxy/internal/logging/memlogger"
	"github.com/yusing/go-proxy/internal/metrics/uptime"
	"github.com/yusing/go-proxy/internal/net/gphttp/servemux"
)

func NewHandler(cfg config.ConfigInstance) http.Handler {
	mux := servemux.NewServeMux(cfg)
	mux.HandleFunc("GET", "/v1", v1.Index)
	mux.HandleFunc("GET", "/v1/version", v1.GetVersion)

	mux.HandleFunc("GET", "/v1/stats", v1.Stats, true)
	mux.HandleFunc("POST", "/v1/reload", v1.Reload, true)
	mux.HandleFunc("GET", "/v1/list", v1.List, true)
	mux.HandleFunc("GET", "/v1/list/{what}", v1.List, true)
	mux.HandleFunc("GET", "/v1/list/{what}/{which}", v1.List, true)
	mux.HandleFunc("GET", "/v1/file/{type}/{filename}", v1.GetFileContent, true)
	mux.HandleFunc("POST,PUT", "/v1/file/{type}/{filename}", v1.SetFileContent, true)
	mux.HandleFunc("POST", "/v1/file/validate/{type}", v1.ValidateFile, true)
	mux.HandleFunc("GET", "/v1/health", v1.Health, true)
	mux.HandleFunc("GET", "/v1/logs", memlogger.Handler(), true)
	mux.HandleFunc("GET", "/v1/favicon", favicon.GetFavIcon, true)
	mux.HandleFunc("POST", "/v1/homepage/set", v1.SetHomePageOverrides, true)
	mux.HandleFunc("GET", "/v1/agents", v1.ListAgents, true)
	mux.HandleFunc("GET", "/v1/agents/new", v1.NewAgent, true)
	mux.HandleFunc("POST", "/v1/agents/verify", v1.VerifyNewAgent, true)
	mux.HandleFunc("GET", "/v1/metrics/system_info", v1.SystemInfo, true)
	mux.HandleFunc("GET", "/v1/metrics/uptime", uptime.Poller.ServeHTTP, true)
	mux.HandleFunc("GET", "/v1/cert/info", certapi.GetCertInfo, true)
	mux.HandleFunc("", "/v1/cert/renew", certapi.RenewCert, true)
	mux.HandleFunc("GET", "/v1/docker/info", dockerapi.DockerInfo, true)
	mux.HandleFunc("GET", "/v1/docker/logs/{server}/{container}", dockerapi.Logs, true)
	mux.HandleFunc("GET", "/v1/docker/containers", dockerapi.Containers, true)

	if common.PrometheusEnabled {
		mux.Handle("GET /v1/metrics", promhttp.Handler())
		logging.Info().Msg("prometheus metrics enabled")
	}

	defaultAuth := auth.GetDefaultAuth()
	if defaultAuth != nil {
		mux.HandleFunc("GET", "/v1/auth/redirect", defaultAuth.RedirectLoginPage)
		mux.HandleFunc("GET", "/v1/auth/check", func(w http.ResponseWriter, r *http.Request) {
			if err := defaultAuth.CheckToken(r); err != nil {
				http.Error(w, err.Error(), http.StatusUnauthorized)
				return
			}
		})
		mux.HandleFunc("GET,POST", "/v1/auth/callback", defaultAuth.LoginCallbackHandler)
		mux.HandleFunc("GET,POST", "/v1/auth/logout", defaultAuth.LogoutCallbackHandler)
	} else {
		mux.HandleFunc("GET", "/v1/auth/check", func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		})
	}
	return mux
}
