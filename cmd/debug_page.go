//go:build !production

package main

import (
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/yusing/godoxy/internal/api"
	apiV1 "github.com/yusing/godoxy/internal/api/v1"
	agentApi "github.com/yusing/godoxy/internal/api/v1/agent"
	authApi "github.com/yusing/godoxy/internal/api/v1/auth"
	certApi "github.com/yusing/godoxy/internal/api/v1/cert"
	dockerApi "github.com/yusing/godoxy/internal/api/v1/docker"
	fileApi "github.com/yusing/godoxy/internal/api/v1/file"
	homepageApi "github.com/yusing/godoxy/internal/api/v1/homepage"
	metricsApi "github.com/yusing/godoxy/internal/api/v1/metrics"
	routeApi "github.com/yusing/godoxy/internal/api/v1/route"
	"github.com/yusing/godoxy/internal/auth"
	"github.com/yusing/godoxy/internal/idlewatcher"
	idlewatcherTypes "github.com/yusing/godoxy/internal/idlewatcher/types"
)

type debugMux struct {
	endpoints []debugEndpoint
	mux       http.ServeMux
}

type debugEndpoint struct {
	name   string
	method string
	path   string
}

func newDebugMux() *debugMux {
	return &debugMux{
		endpoints: make([]debugEndpoint, 0),
		mux:       *http.NewServeMux(),
	}
}

func (mux *debugMux) registerEndpoint(name, method, path string) {
	mux.endpoints = append(mux.endpoints, debugEndpoint{name: name, method: method, path: path})
}

func (mux *debugMux) HandleFunc(name, method, path string, handler http.HandlerFunc) {
	mux.registerEndpoint(name, method, path)
	mux.mux.HandleFunc(method+" "+path, handler)
}

func (mux *debugMux) Finalize() {
	mux.mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		fmt.Fprintln(w, `
<!DOCTYPE html>
<html>
	<head>
		<style>
				body {
					font-family: ui-sans-serif, system-ui, -apple-system, Segoe UI, Roboto, Helvetica, Arial, Apple Color Emoji, Segoe UI Emoji;
					font-size: 16px;
					line-height: 1.5;
					color: #f8f9fa;
					background-color: #121212;
					margin: 0;
					padding: 0;
				}
				table {
					border-collapse: collapse;
					width: 100%;
					margin-top: 20px;
				}
				th, td {
					padding: 12px;
					text-align: left;
					border-bottom: 1px solid #333;
				}
				th {
					background-color: #1e1e1e;
					font-weight: 600;
					color: #f8f9fa;
				}
				td {
					color: #e9ecef;
				}
				.link {
					color: #007bff;
					text-decoration: none;
				}
				.link:hover {
					text-decoration: underline;
				}
				.method {
					color: #6c757d;
					font-family: monospace;
				}
				.path {
					color: #6c757d;
					font-family: monospace;
				}
		</style>
	</head>
	<body>
		<table>
			<thead>
				<tr>
					<th>Name</th>
					<th>Method</th>
					<th>Path</th>
				</tr>
			</thead>
			<tbody>`)
		for _, endpoint := range mux.endpoints {
			fmt.Fprintf(w, "<tr><td><a class='link' href=%q>%s</a></td><td class='method'>%s</td><td class='path'>%s</td></tr>", endpoint.path, endpoint.name, endpoint.method, endpoint.path)
		}
		fmt.Fprintln(w, `
			</tbody>
		</table>
	</body>
</html>`)
	})
}

func listenDebugServer() {
	mux := newDebugMux()
	mux.mux.HandleFunc("/favicon.ico", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/svg+xml")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 100 100"><text x="50" y="50" text-anchor="middle" dominant-baseline="middle">🐙</text></svg>`))
	})

	mux.HandleFunc("Auth block page", "GET", "/auth/block", AuthBlockPageHandler)
	mux.HandleFunc("Idlewatcher loading page", "GET", idlewatcherTypes.PathPrefix, idlewatcher.DebugHandler)
	apiHandler := newApiHandler(mux)
	mux.mux.HandleFunc("/api/v1/", apiHandler.ServeHTTP)

	mux.Finalize()

	go http.ListenAndServe(":7777", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Pragma", "no-cache")
		w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")
		w.Header().Set("Expires", "0")
		mux.mux.ServeHTTP(w, r)
	}))
}

func newApiHandler(debugMux *debugMux) *gin.Engine {
	r := gin.New()
	r.Use(api.ErrorHandler())
	r.Use(api.ErrorLoggingMiddleware())
	r.Use(api.NoCache())

	registerGinRoute := func(router gin.IRouter, method, name string, path string, handler gin.HandlerFunc) {
		if group, ok := router.(*gin.RouterGroup); ok {
			debugMux.registerEndpoint(name, method, group.BasePath()+path)
		} else {
			debugMux.registerEndpoint(name, method, path)
		}
		router.Handle(method, path, handler)
	}

	registerGinRoute(r, "GET", "App version", "/api/v1/version", apiV1.Version)

	v1 := r.Group("/api/v1")
	if auth.IsEnabled() {
		v1Auth := v1.Group("/auth")
		{
			registerGinRoute(v1Auth, "HEAD", "Auth check", "/check", authApi.Check)
			registerGinRoute(v1Auth, "POST", "Auth login", "/login", authApi.Login)
			registerGinRoute(v1Auth, "GET", "Auth callback", "/callback", authApi.Callback)
			registerGinRoute(v1Auth, "POST", "Auth callback", "/callback", authApi.Callback)
			registerGinRoute(v1Auth, "POST", "Auth logout", "/logout", authApi.Logout)
			registerGinRoute(v1Auth, "GET", "Auth logout", "/logout", authApi.Logout)
		}
	}

	{
		// enable cache for favicon
		registerGinRoute(v1, "GET", "Route favicon", "/favicon", apiV1.FavIcon)
		registerGinRoute(v1, "GET", "Route health", "/health", apiV1.Health)
		registerGinRoute(v1, "GET", "List icons", "/icons", apiV1.Icons)
		registerGinRoute(v1, "POST", "Config reload", "/reload", apiV1.Reload)
		registerGinRoute(v1, "GET", "Route stats", "/stats", apiV1.Stats)

		route := v1.Group("/route")
		{
			registerGinRoute(route, "GET", "List routes", "/list", routeApi.Routes)
			registerGinRoute(route, "GET", "Get route", "/:which", routeApi.Route)
			registerGinRoute(route, "GET", "List providers", "/providers", routeApi.Providers)
			registerGinRoute(route, "GET", "List routes by provider", "/by_provider", routeApi.ByProvider)
			registerGinRoute(route, "POST", "Playground", "/playground", routeApi.Playground)
		}

		file := v1.Group("/file")
		{
			registerGinRoute(file, "GET", "List files", "/list", fileApi.List)
			registerGinRoute(file, "GET", "Get file", "/content", fileApi.Get)
			registerGinRoute(file, "PUT", "Set file", "/content", fileApi.Set)
			registerGinRoute(file, "POST", "Set file", "/content", fileApi.Set)
			registerGinRoute(file, "POST", "Validate file", "/validate", fileApi.Validate)
		}

		homepage := v1.Group("/homepage")
		{
			registerGinRoute(homepage, "GET", "List categories", "/categories", homepageApi.Categories)
			registerGinRoute(homepage, "GET", "List items", "/items", homepageApi.Items)
			registerGinRoute(homepage, "POST", "Set item", "/set/item", homepageApi.SetItem)
			registerGinRoute(homepage, "POST", "Set items batch", "/set/items_batch", homepageApi.SetItemsBatch)
			registerGinRoute(homepage, "POST", "Set item visible", "/set/item_visible", homepageApi.SetItemVisible)
			registerGinRoute(homepage, "POST", "Set item favorite", "/set/item_favorite", homepageApi.SetItemFavorite)
			registerGinRoute(homepage, "POST", "Set item sort order", "/set/item_sort_order", homepageApi.SetItemSortOrder)
			registerGinRoute(homepage, "POST", "Set item all sort order", "/set/item_all_sort_order", homepageApi.SetItemAllSortOrder)
			registerGinRoute(homepage, "POST", "Set item fav sort order", "/set/item_fav_sort_order", homepageApi.SetItemFavSortOrder)
			registerGinRoute(homepage, "POST", "Set category order", "/set/category_order", homepageApi.SetCategoryOrder)
			registerGinRoute(homepage, "POST", "Item click", "/item_click", homepageApi.ItemClick)
		}

		cert := v1.Group("/cert")
		{
			registerGinRoute(cert, "GET", "Get cert info", "/info", certApi.Info)
			registerGinRoute(cert, "GET", "Renew cert", "/renew", certApi.Renew)
		}

		agent := v1.Group("/agent")
		{
			registerGinRoute(agent, "GET", "List agents", "/list", agentApi.List)
			registerGinRoute(agent, "POST", "Create agent", "/create", agentApi.Create)
			registerGinRoute(agent, "POST", "Verify agent", "/verify", agentApi.Verify)
		}

		metrics := v1.Group("/metrics")
		{
			registerGinRoute(metrics, "GET", "Get system info", "/system_info", metricsApi.SystemInfo)
			registerGinRoute(metrics, "GET", "Get all system info", "/all_system_info", metricsApi.AllSystemInfo)
			registerGinRoute(metrics, "GET", "Get uptime", "/uptime", metricsApi.Uptime)
		}

		docker := v1.Group("/docker")
		{
			registerGinRoute(docker, "GET", "Get container", "/container/:id", dockerApi.GetContainer)
			registerGinRoute(docker, "GET", "List containers", "/containers", dockerApi.Containers)
			registerGinRoute(docker, "GET", "Get docker info", "/info", dockerApi.Info)
			registerGinRoute(docker, "GET", "Get docker logs", "/logs/:id", dockerApi.Logs)
			registerGinRoute(docker, "POST", "Start docker container", "/start", dockerApi.Start)
			registerGinRoute(docker, "POST", "Stop docker container", "/stop", dockerApi.Stop)
			registerGinRoute(docker, "POST", "Restart docker container", "/restart", dockerApi.Restart)
		}
	}

	return r
}

func AuthBlockPageHandler(w http.ResponseWriter, r *http.Request) {
	auth.WriteBlockPage(w, http.StatusForbidden, "Forbidden", "Login", "/login")
}
