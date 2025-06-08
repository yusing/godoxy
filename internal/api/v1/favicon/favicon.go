package favicon

import (
	"net/http"

	"github.com/yusing/go-proxy/internal/homepage"
	"github.com/yusing/go-proxy/internal/net/gphttp"
	"github.com/yusing/go-proxy/internal/route/routes"
)

// GetFavIcon returns the favicon of the route
//
// Returns:
//   - 200 OK: if icon found
//   - 400 Bad Request: if alias is empty or route is not HTTPRoute
//   - 404 Not Found: if route or icon not found
//   - 500 Internal Server Error: if internal error
//   - others: depends on route handler response
func GetFavIcon(w http.ResponseWriter, req *http.Request) {
	url, alias := req.FormValue("url"), req.FormValue("alias")
	if url == "" && alias == "" {
		gphttp.MissingKey(w, "url or alias")
		return
	}
	if url != "" && alias != "" {
		gphttp.BadRequest(w, "url and alias are mutually exclusive")
		return
	}

	// try with url
	if url != "" {
		var iconURL homepage.IconURL
		if err := iconURL.Parse(url); err != nil {
			gphttp.ClientError(w, req, err, http.StatusBadRequest)
			return
		}
		fetchResult := homepage.FetchFavIconFromURL(req.Context(), &iconURL)
		if !fetchResult.OK() {
			http.Error(w, fetchResult.ErrMsg, fetchResult.StatusCode)
			return
		}
		w.Header().Set("Content-Type", fetchResult.ContentType())
		gphttp.WriteBody(w, fetchResult.Icon)
		return
	}

	// try with alias
	GetFavIconFromAlias(w, req, alias)
	return
}

func GetFavIconFromAlias(w http.ResponseWriter, req *http.Request, alias string) {
	// try with route.Icon
	r, ok := routes.HTTP.Get(alias)
	if !ok {
		gphttp.ValueNotFound(w, "route", alias)
		return
	}

	var result *homepage.FetchResult
	hp := r.HomepageItem()
	if hp.Icon != nil {
		if hp.Icon.IconSource == homepage.IconSourceRelative {
			result = homepage.FindIcon(req.Context(), r, *hp.Icon.FullURL)
		} else {
			result = homepage.FetchFavIconFromURL(req.Context(), hp.Icon)
		}
	} else {
		// try extract from "link[rel=icon]"
		result = homepage.FindIcon(req.Context(), r, "/")
	}
	if result.StatusCode == 0 {
		result.StatusCode = http.StatusOK
	}
	if !result.OK() {
		http.Error(w, result.ErrMsg, result.StatusCode)
		return
	}
	w.Header().Set("Content-Type", result.ContentType())
	gphttp.WriteBody(w, result.Icon)
}
