package favicon

import (
	"errors"
	"net/http"

	"github.com/yusing/go-proxy/internal/gperr"
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
		gphttp.ClientError(w, gphttp.ErrMissingKey("url or alias"), http.StatusBadRequest)
		return
	}
	if url != "" && alias != "" {
		gphttp.ClientError(w, gperr.New("url and alias are mutually exclusive"), http.StatusBadRequest)
		return
	}

	// try with url
	if url != "" {
		var iconURL homepage.IconURL
		if err := iconURL.Parse(url); err != nil {
			gphttp.ClientError(w, err, http.StatusBadRequest)
			return
		}
		fetchResult := homepage.FetchFavIconFromURL(&iconURL)
		if !fetchResult.OK() {
			http.Error(w, fetchResult.ErrMsg, fetchResult.StatusCode)
			return
		}
		w.Header().Set("Content-Type", fetchResult.ContentType())
		gphttp.WriteBody(w, fetchResult.Icon)
		return
	}

	// try with route.Icon
	r, ok := routes.GetHTTPRoute(alias)
	if !ok {
		gphttp.ClientError(w, errors.New("no such route"), http.StatusNotFound)
		return
	}

	var result *homepage.FetchResult
	hp := r.HomepageItem()
	if hp.Icon != nil {
		if hp.Icon.IconSource == homepage.IconSourceRelative {
			result = homepage.FindIcon(req.Context(), r, hp.Icon.Value)
		} else {
			result = homepage.FetchFavIconFromURL(hp.Icon)
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
