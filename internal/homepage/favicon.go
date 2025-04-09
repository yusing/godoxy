package homepage

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"
	"github.com/vincent-petithory/dataurl"
	gphttp "github.com/yusing/go-proxy/internal/net/gphttp"
	"github.com/yusing/go-proxy/internal/utils/strutils"
)

type FetchResult struct {
	Icon       []byte
	StatusCode int
	ErrMsg     string

	contentType string
}

func (res *FetchResult) OK() bool {
	return res.Icon != nil
}

func (res *FetchResult) ContentType() string {
	if res.contentType == "" {
		if bytes.HasPrefix(res.Icon, []byte("<svg")) || bytes.HasPrefix(res.Icon, []byte("<?xml")) {
			return "image/svg+xml"
		}
		return "image/x-icon"
	}
	return res.contentType
}

const maxRedirectDepth = 5

func FetchFavIconFromURL(iconURL *IconURL) *FetchResult {
	switch iconURL.IconSource {
	case IconSourceAbsolute:
		return fetchIconAbsolute(iconURL.URL())
	case IconSourceRelative:
		return &FetchResult{StatusCode: http.StatusBadRequest, ErrMsg: "unexpected relative icon"}
	case IconSourceWalkXCode, IconSourceSelfhSt:
		return fetchKnownIcon(iconURL)
	}
	return &FetchResult{StatusCode: http.StatusBadRequest, ErrMsg: "invalid icon source"}
}

func fetchIconAbsolute(url string) *FetchResult {
	if result := loadIconCache(url); result != nil {
		return result
	}

	resp, err := gphttp.Get(url)
	if err != nil || resp.StatusCode != http.StatusOK {
		if err == nil {
			err = errors.New(resp.Status)
		}
		return &FetchResult{StatusCode: http.StatusBadGateway, ErrMsg: "connection error"}
	}

	defer resp.Body.Close()
	icon, err := io.ReadAll(resp.Body)
	if err != nil {
		return &FetchResult{StatusCode: http.StatusInternalServerError, ErrMsg: "internal error"}
	}

	storeIconCache(url, icon)
	return &FetchResult{Icon: icon}
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

func fetchKnownIcon(url *IconURL) *FetchResult {
	// if icon isn't in the list, no need to fetch
	if !url.HasIcon() {
		return &FetchResult{StatusCode: http.StatusNotFound, ErrMsg: "no such icon"}
	}

	return fetchIconAbsolute(url.URL())
}

func fetchIcon(filetype, filename string) *FetchResult {
	result := fetchKnownIcon(NewSelfhStIconURL(filename, filetype))
	if result.Icon == nil {
		return result
	}
	return fetchKnownIcon(NewWalkXCodeIconURL(filename, filetype))
}

func FindIcon(ctx context.Context, r route, uri string) *FetchResult {
	key := routeKey(r)
	if result := loadIconCache(key); result != nil {
		return result
	}

	result := fetchIcon("png", sanitizeName(r.Reference()))
	if !result.OK() {
		if r, ok := r.(httpRoute); ok {
			// fallback to parse html
			result = findIconSlow(ctx, r, uri, 0)
		}
	}
	if result.OK() {
		storeIconCache(key, result.Icon)
	}
	return result
}

func findIconSlow(ctx context.Context, r httpRoute, uri string, depth int) *FetchResult {
	ctx, cancel := context.WithTimeoutCause(ctx, 3*time.Second, errors.New("favicon request timeout"))
	defer cancel()

	newReq, err := http.NewRequestWithContext(ctx, "GET", r.TargetURL().String(), nil)
	if err != nil {
		return &FetchResult{StatusCode: http.StatusInternalServerError, ErrMsg: "cannot create request"}
	}
	newReq.Header.Set("Accept-Encoding", "identity") // disable compression

	u, err := url.ParseRequestURI(strutils.SanitizeURI(uri))
	if err != nil {
		return &FetchResult{StatusCode: http.StatusInternalServerError, ErrMsg: "cannot parse uri"}
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
			return &FetchResult{StatusCode: http.StatusBadGateway, ErrMsg: "connection error"}
		default:
			if loc := c.Header().Get("Location"); loc != "" {
				if depth > maxRedirectDepth {
					return &FetchResult{StatusCode: http.StatusBadGateway, ErrMsg: "too many redirects"}
				}
				loc = strutils.SanitizeURI(loc)
				if loc == "/" || loc == newReq.URL.Path {
					return &FetchResult{StatusCode: http.StatusBadGateway, ErrMsg: "circular redirect"}
				}
				return findIconSlow(ctx, r, loc, depth+1)
			}
		}
		return &FetchResult{StatusCode: c.status, ErrMsg: "upstream error: " + string(c.data)}
	}
	// return icon data
	if !gphttp.GetContentType(c.header).IsHTML() {
		return &FetchResult{Icon: c.data, contentType: c.header.Get("Content-Type")}
	}
	// try extract from "link[rel=icon]" from path "/"
	doc, err := goquery.NewDocumentFromReader(bytes.NewBuffer(c.data))
	if err != nil {
		return &FetchResult{StatusCode: http.StatusInternalServerError, ErrMsg: "failed to parse html"}
	}
	ele := doc.Find("head > link[rel=icon]").First()
	if ele.Length() == 0 {
		return &FetchResult{StatusCode: http.StatusNotFound, ErrMsg: "icon element not found"}
	}
	href := ele.AttrOr("href", "")
	if href == "" {
		return &FetchResult{StatusCode: http.StatusNotFound, ErrMsg: "icon href not found"}
	}
	// https://en.wikipedia.org/wiki/Data_URI_scheme
	if strings.HasPrefix(href, "data:image/") {
		dataURI, err := dataurl.DecodeString(href)
		if err != nil {
			return &FetchResult{StatusCode: http.StatusInternalServerError, ErrMsg: "failed to decode favicon"}
		}
		return &FetchResult{Icon: dataURI.Data, contentType: dataURI.ContentType()}
	}
	switch {
	case strings.HasPrefix(href, "http://"), strings.HasPrefix(href, "https://"):
		return fetchIconAbsolute(href)
	default:
		return findIconSlow(ctx, r, href, 0)
	}
}
