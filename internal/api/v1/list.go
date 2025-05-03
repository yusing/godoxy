package v1

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/yusing/go-proxy/internal/common"
	config "github.com/yusing/go-proxy/internal/config/types"
	"github.com/yusing/go-proxy/internal/homepage"
	"github.com/yusing/go-proxy/internal/net/gphttp"
	"github.com/yusing/go-proxy/internal/net/gphttp/middleware"
	"github.com/yusing/go-proxy/internal/route/routes"
	route "github.com/yusing/go-proxy/internal/route/types"
	"github.com/yusing/go-proxy/internal/task"
	"github.com/yusing/go-proxy/internal/utils"
)

const (
	ListRoute              = "route"
	ListRoutes             = "routes"
	ListFiles              = "files"
	ListMiddlewares        = "middlewares"
	ListMiddlewareTraces   = "middleware_trace"
	ListMatchDomains       = "match_domains"
	ListHomepageConfig     = "homepage_config"
	ListRouteProviders     = "route_providers"
	ListHomepageCategories = "homepage_categories"
	ListIcons              = "icons"
	ListTasks              = "tasks"
)

func List(cfg config.ConfigInstance, w http.ResponseWriter, r *http.Request) {
	what := r.PathValue("what")
	if what == "" {
		what = ListRoutes
	}
	which := r.PathValue("which")

	switch what {
	case ListRoute:
		route := listRoute(which)
		if route == nil {
			http.NotFound(w, r)
		} else {
			gphttp.RespondJSON(w, r, route)
		}
	case ListRoutes:
		gphttp.RespondJSON(w, r, routes.ByAlias(route.RouteType(r.FormValue("type"))))
	case ListFiles:
		listFiles(w, r)
	case ListMiddlewares:
		gphttp.RespondJSON(w, r, middleware.All())
	case ListMiddlewareTraces:
		gphttp.RespondJSON(w, r, middleware.GetAllTrace())
	case ListMatchDomains:
		gphttp.RespondJSON(w, r, cfg.Value().MatchDomains)
	case ListHomepageConfig:
		gphttp.RespondJSON(w, r, routes.HomepageConfig(r.FormValue("category"), r.FormValue("provider")))
	case ListRouteProviders:
		gphttp.RespondJSON(w, r, cfg.RouteProviderList())
	case ListHomepageCategories:
		gphttp.RespondJSON(w, r, routes.HomepageCategories())
	case ListIcons:
		limit, err := strconv.Atoi(r.FormValue("limit"))
		if err != nil {
			limit = 0
		}
		icons, err := homepage.SearchIcons(r.FormValue("keyword"), limit)
		if err != nil {
			gphttp.ClientError(w, r, err)
			return
		}
		gphttp.RespondJSON(w, r, icons)
	case ListTasks:
		gphttp.RespondJSON(w, r, task.DebugTaskList())
	default:
		gphttp.BadRequest(w, fmt.Sprintf("invalid what: %s", what))
	}
}

// if which is "all" or empty, return map[string]Route of all routes
// otherwise, return a single Route with alias which or nil if not found.
func listRoute(which string) any {
	if which == "" || which == "all" {
		return routes.ByAlias()
	}
	routes := routes.ByAlias()
	route, ok := routes[which]
	if !ok {
		return nil
	}
	return route
}

func listFiles(w http.ResponseWriter, r *http.Request) {
	files, err := utils.ListFiles(common.ConfigBasePath, 0, true)
	if err != nil {
		gphttp.ServerError(w, r, err)
		return
	}
	resp := map[FileType][]string{
		FileTypeConfig:     make([]string, 0),
		FileTypeProvider:   make([]string, 0),
		FileTypeMiddleware: make([]string, 0),
	}

	for _, file := range files {
		t := fileType(file)
		file = strings.TrimPrefix(file, common.ConfigBasePath+"/")
		resp[t] = append(resp[t], file)
	}

	mids, err := utils.ListFiles(common.MiddlewareComposeBasePath, 0, true)
	if err != nil {
		gphttp.ServerError(w, r, err)
		return
	}
	for _, mid := range mids {
		mid = strings.TrimPrefix(mid, common.MiddlewareComposeBasePath+"/")
		resp[FileTypeMiddleware] = append(resp[FileTypeMiddleware], mid)
	}
	gphttp.RespondJSON(w, r, resp)
}
