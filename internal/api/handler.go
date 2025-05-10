package api

import (
	"fmt"
	"net/http"

	v1 "github.com/yusing/go-proxy/internal/api/v1"
	"github.com/yusing/go-proxy/internal/api/v1/certapi"
	"github.com/yusing/go-proxy/internal/api/v1/dockerapi"
	"github.com/yusing/go-proxy/internal/api/v1/favicon"
	"github.com/yusing/go-proxy/internal/auth"
	config "github.com/yusing/go-proxy/internal/config/types"
	"github.com/yusing/go-proxy/internal/logging/memlogger"
	"github.com/yusing/go-proxy/internal/metrics/uptime"
	"github.com/yusing/go-proxy/internal/net/gphttp/gpwebsocket"
	"github.com/yusing/go-proxy/internal/net/gphttp/httpheaders"
	"github.com/yusing/go-proxy/internal/utils/strutils"
	"github.com/yusing/go-proxy/pkg"
)

type (
	ServeMux struct {
		*http.ServeMux
		cfg config.ConfigInstance
	}
	WithCfgHandler = func(config.ConfigInstance, http.ResponseWriter, *http.Request)
)

func (mux ServeMux) HandleFunc(methods, endpoint string, h any, requireAuth ...bool) {
	var handler http.HandlerFunc
	switch h := h.(type) {
	case func(http.ResponseWriter, *http.Request):
		handler = h
	case http.Handler:
		handler = h.ServeHTTP
	case WithCfgHandler:
		handler = func(w http.ResponseWriter, r *http.Request) {
			h(mux.cfg, w, r)
		}
	default:
		panic(fmt.Errorf("unsupported handler type: %T", h))
	}

	matchDomains := mux.cfg.Value().MatchDomains
	if len(matchDomains) > 0 {
		origHandler := handler
		handler = func(w http.ResponseWriter, r *http.Request) {
			if httpheaders.IsWebsocket(r.Header) {
				gpwebsocket.SetWebsocketAllowedDomains(r.Header, matchDomains)
			}
			origHandler(w, r)
		}
	}

	if len(requireAuth) > 0 && requireAuth[0] {
		handler = auth.RequireAuth(handler)
	}
	if methods == "" {
		mux.ServeMux.HandleFunc(endpoint, handler)
	} else {
		for _, m := range strutils.CommaSeperatedList(methods) {
			mux.ServeMux.HandleFunc(m+" "+endpoint, handler)
		}
	}
}

func NewHandler(cfg config.ConfigInstance) http.Handler {
	mux := ServeMux{http.NewServeMux(), cfg}
	mux.HandleFunc("GET", "/v1", v1.Index)
	mux.HandleFunc("GET", "/v1/version", pkg.GetVersionHTTPHandler())

	mux.HandleFunc("GET", "/v1/stats", v1.Stats, true)
	mux.HandleFunc("POST", "/v1/reload", v1.Reload, true)
	mux.HandleFunc("GET", "/v1/list", v1.ListRoutesHandler, true)
	mux.HandleFunc("GET", "/v1/list/routes", v1.ListRoutesHandler, true)
	mux.HandleFunc("GET", "/v1/list/route/{which}", v1.ListRouteHandler, true)
	mux.HandleFunc("GET", "/v1/list/routes_by_provider", v1.ListRoutesByProviderHandler, true)
	mux.HandleFunc("GET", "/v1/list/files", v1.ListFilesHandler, true)
	mux.HandleFunc("GET", "/v1/list/homepage_config", v1.ListHomepageConfigHandler, true)
	mux.HandleFunc("GET", "/v1/list/route_providers", v1.ListRouteProvidersHandler, true)
	mux.HandleFunc("GET", "/v1/list/homepage_categories", v1.ListHomepageCategoriesHandler, true)
	mux.HandleFunc("GET", "/v1/list/icons", v1.ListIconsHandler, true)
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

	defaultAuth := auth.GetDefaultAuth()
	if defaultAuth == nil {
		return mux
	}

	mux.HandleFunc("GET", "/v1/auth/check", auth.AuthCheckHandler)
	mux.HandleFunc("GET,POST", "/v1/auth/redirect", defaultAuth.LoginHandler)
	mux.HandleFunc("GET,POST", "/v1/auth/callback", defaultAuth.PostAuthCallbackHandler)
	mux.HandleFunc("GET,POST", "/v1/auth/logout", defaultAuth.LogoutHandler)
	return mux
}
