package favicon

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"net/url"
	"path"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"
	"github.com/vincent-petithory/dataurl"
	"github.com/yusing/go-proxy/internal/gperr"
	"github.com/yusing/go-proxy/internal/homepage"
	"github.com/yusing/go-proxy/internal/logging"
	gphttp "github.com/yusing/go-proxy/internal/net/gphttp"
	"github.com/yusing/go-proxy/internal/route/routes"
	route "github.com/yusing/go-proxy/internal/route/types"
	"github.com/yusing/go-proxy/internal/utils/strutils"
)

type fetchResult struct {
	icon        []byte
	contentType string
	statusCode  int
	errMsg      string
}

func (res *fetchResult) OK() bool {
	return res.icon != nil
}

func (res *fetchResult) ContentType() string {
	if res.contentType == "" {
		if bytes.HasPrefix(res.icon, []byte("<svg")) || bytes.HasPrefix(res.icon, []byte("<?xml")) {
			return "image/svg+xml"
		}
		return "image/x-icon"
	}
	return res.contentType
}

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
		fetchResult := getFavIconFromURL(&iconURL)
		if !fetchResult.OK() {
			http.Error(w, fetchResult.errMsg, fetchResult.statusCode)
			return
		}
		w.Header().Set("Content-Type", fetchResult.ContentType())
		gphttp.WriteBody(w, fetchResult.icon)
		return
	}

	// try with route.Homepage.Icon
	r, ok := routes.GetHTTPRoute(alias)
	if !ok {
		gphttp.ClientError(w, errors.New("no such route"), http.StatusNotFound)
		return
	}

	var result *fetchResult
	hp := r.HomepageItem()
	if hp.Icon != nil {
		if hp.Icon.IconSource == homepage.IconSourceRelative {
			result = findIcon(r, req, hp.Icon.Value)
		} else {
			result = getFavIconFromURL(hp.Icon)
		}
	} else {
		// try extract from "link[rel=icon]"
		result = findIcon(r, req, "/")
	}
	if result.statusCode == 0 {
		result.statusCode = http.StatusOK
	}
	if !result.OK() {
		http.Error(w, result.errMsg, result.statusCode)
		return
	}
	w.Header().Set("Content-Type", result.ContentType())
	gphttp.WriteBody(w, result.icon)
}

func getFavIconFromURL(iconURL *homepage.IconURL) *fetchResult {
	switch iconURL.IconSource {
	case homepage.IconSourceAbsolute:
		return fetchIconAbsolute(iconURL.URL())
	case homepage.IconSourceRelative:
		return &fetchResult{statusCode: http.StatusBadRequest, errMsg: "unexpected relative icon"}
	case homepage.IconSourceWalkXCode, homepage.IconSourceSelfhSt:
		return fetchKnownIcon(iconURL)
	}
	return &fetchResult{statusCode: http.StatusBadRequest, errMsg: "invalid icon source"}
}

func fetchIconAbsolute(url string) *fetchResult {
	if result := loadIconCache(url); result != nil {
		return result
	}

	resp, err := gphttp.Get(url)
	if err != nil || resp.StatusCode != http.StatusOK {
		if err == nil {
			err = errors.New(resp.Status)
		}
		logging.Error().Err(err).
			Str("url", url).
			Msg("failed to get icon")
		return &fetchResult{statusCode: http.StatusBadGateway, errMsg: "connection error"}
	}

	defer resp.Body.Close()
	icon, err := io.ReadAll(resp.Body)
	if err != nil {
		logging.Error().Err(err).
			Str("url", url).
			Msg("failed to read icon")
		return &fetchResult{statusCode: http.StatusInternalServerError, errMsg: "internal error"}
	}

	storeIconCache(url, icon)
	return &fetchResult{icon: icon}
}

var nameSanitizer = strings.NewReplacer(
	"_", "-",
	" ", "-",
	"(", "",
	")", "",
)

func sanitizeName(name string) string {
	return strings.ToLower(nameSanitizer.Replace(name))
}

func fetchKnownIcon(url *homepage.IconURL) *fetchResult {
	// if icon isn't in the list, no need to fetch
	if !url.HasIcon() {
		logging.Debug().
			Str("value", url.String()).
			Str("url", url.URL()).
			Msg("no such icon")
		return &fetchResult{statusCode: http.StatusNotFound, errMsg: "no such icon"}
	}

	return fetchIconAbsolute(url.URL())
}

func fetchIcon(filetype, filename string) *fetchResult {
	result := fetchKnownIcon(homepage.NewSelfhStIconURL(filename, filetype))
	if result.icon == nil {
		return result
	}
	return fetchKnownIcon(homepage.NewWalkXCodeIconURL(filename, filetype))
}

func findIcon(r route.HTTPRoute, req *http.Request, uri string) *fetchResult {
	key := routeKey(r)
	if result := loadIconCache(key); result != nil {
		return result
	}

	result := fetchIcon("png", sanitizeName(r.TargetName()))
	cont := r.ContainerInfo()
	if !result.OK() && cont != nil {
		result = fetchIcon("png", sanitizeName(cont.Image.Name))
	}
	if !result.OK() {
		// fallback to parse html
		result = findIconSlow(r, req, uri)
	}
	if result.OK() {
		storeIconCache(key, result.icon)
	}
	return result
}

func findIconSlow(r route.HTTPRoute, req *http.Request, uri string) *fetchResult {
	ctx, cancel := context.WithTimeoutCause(req.Context(), 3*time.Second, errors.New("favicon request timeout"))
	defer cancel()
	newReq := req.WithContext(ctx)
	newReq.Header.Set("Accept-Encoding", "identity") // disable compression
	u, err := url.ParseRequestURI(strutils.SanitizeURI(uri))
	if err != nil {
		logging.Error().Err(err).
			Str("route", r.TargetName()).
			Str("path", uri).
			Msg("failed to parse uri")
		return &fetchResult{statusCode: http.StatusInternalServerError, errMsg: "cannot parse uri"}
	}
	newReq.URL.Path = u.Path
	newReq.URL.RawPath = u.RawPath
	newReq.URL.RawQuery = u.RawQuery
	newReq.RequestURI = u.String()

	c := newContent()
	r.ServeHTTP(c, newReq)
	if c.status != http.StatusOK {
		switch c.status {
		case 0:
			return &fetchResult{statusCode: http.StatusBadGateway, errMsg: "connection error"}
		default:
			if loc := c.Header().Get("Location"); loc != "" {
				loc = strutils.SanitizeURI(loc)
				if loc == "/" || loc == newReq.URL.Path {
					return &fetchResult{statusCode: http.StatusBadGateway, errMsg: "circular redirect"}
				}
				return findIconSlow(r, req, loc)
			}
		}
		return &fetchResult{statusCode: c.status, errMsg: "upstream error: " + string(c.data)}
	}
	// return icon data
	if !gphttp.GetContentType(c.header).IsHTML() {
		return &fetchResult{icon: c.data, contentType: c.header.Get("Content-Type")}
	}
	// try extract from "link[rel=icon]" from path "/"
	doc, err := goquery.NewDocumentFromReader(bytes.NewBuffer(c.data))
	if err != nil {
		logging.Error().Err(err).
			Str("route", r.TargetName()).
			Msg("failed to parse html")
		return &fetchResult{statusCode: http.StatusInternalServerError, errMsg: "internal error"}
	}
	ele := doc.Find("head > link[rel=icon]").First()
	if ele.Length() == 0 {
		return &fetchResult{statusCode: http.StatusNotFound, errMsg: "icon element not found"}
	}
	href := ele.AttrOr("href", "")
	if href == "" {
		return &fetchResult{statusCode: http.StatusNotFound, errMsg: "icon href not found"}
	}
	// https://en.wikipedia.org/wiki/Data_URI_scheme
	if strings.HasPrefix(href, "data:image/") {
		dataURI, err := dataurl.DecodeString(href)
		if err != nil {
			logging.Error().Err(err).
				Str("route", r.TargetName()).
				Msg("failed to decode favicon")
			return &fetchResult{statusCode: http.StatusInternalServerError, errMsg: "internal error"}
		}
		return &fetchResult{icon: dataURI.Data, contentType: dataURI.ContentType()}
	}
	switch {
	case strings.HasPrefix(href, "http://"), strings.HasPrefix(href, "https://"):
		return fetchIconAbsolute(href)
	default:
		return findIconSlow(r, req, path.Clean(href))
	}
}
