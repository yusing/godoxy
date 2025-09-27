package homepage

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"
	"github.com/vincent-petithory/dataurl"
	gphttp "github.com/yusing/godoxy/internal/net/gphttp"
	strutils "github.com/yusing/goutils/strings"
)

type FetchResult struct {
	Icon       []byte
	StatusCode int
	ErrMsg     string

	contentType string
}

const faviconFetchTimeout = 3 * time.Second

func (res *FetchResult) OK() bool {
	return len(res.Icon) > 0
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

func FetchFavIconFromURL(ctx context.Context, iconURL *IconURL) *FetchResult {
	switch iconURL.IconSource {
	case IconSourceAbsolute:
		return fetchIconAbsolute(ctx, iconURL.URL())
	case IconSourceRelative:
		return &FetchResult{StatusCode: http.StatusBadRequest, ErrMsg: "unexpected relative icon"}
	case IconSourceWalkXCode, IconSourceSelfhSt:
		return fetchKnownIcon(ctx, iconURL)
	}
	return &FetchResult{StatusCode: http.StatusBadRequest, ErrMsg: "invalid icon source"}
}

func fetchIconAbsolute(ctx context.Context, url string) *FetchResult {
	if result := loadIconCache(url); result != nil {
		return result
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
			return &FetchResult{StatusCode: http.StatusBadGateway, ErrMsg: "request timeout"}
		}
		return &FetchResult{StatusCode: http.StatusInternalServerError, ErrMsg: err.Error()}
	}

	resp, err := gphttp.Do(req)
	if err == nil {
		defer resp.Body.Close()
	}
	if err != nil || resp.StatusCode != http.StatusOK {
		return &FetchResult{StatusCode: http.StatusBadGateway, ErrMsg: "connection error"}
	}

	icon, err := io.ReadAll(resp.Body)
	if err != nil {
		return &FetchResult{StatusCode: http.StatusInternalServerError, ErrMsg: "internal error"}
	}

	if len(icon) == 0 {
		return &FetchResult{StatusCode: http.StatusNotFound, ErrMsg: "empty icon"}
	}

	res := &FetchResult{Icon: icon}
	if contentType := resp.Header.Get("Content-Type"); contentType != "" {
		res.contentType = contentType
	}
	// else leave it empty
	storeIconCache(url, res)
	return res
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

func fetchKnownIcon(ctx context.Context, url *IconURL) *FetchResult {
	// if icon isn't in the list, no need to fetch
	if !url.HasIcon() {
		return &FetchResult{StatusCode: http.StatusNotFound, ErrMsg: "no such icon"}
	}

	return fetchIconAbsolute(ctx, url.URL())
}

func fetchIcon(ctx context.Context, filename string) *FetchResult {
	for _, fileType := range []string{"svg", "webp", "png"} {
		result := fetchKnownIcon(ctx, NewSelfhStIconURL(filename, fileType))
		if result.OK() {
			return result
		}
		result = fetchKnownIcon(ctx, NewWalkXCodeIconURL(filename, fileType))
		if result.OK() {
			return result
		}
	}
	return &FetchResult{StatusCode: http.StatusNotFound, ErrMsg: "no icon found"}
}

func FindIcon(ctx context.Context, r route, uri string) *FetchResult {
	if result := loadIconCache(r.Key()); result != nil {
		return result
	}

	for _, ref := range r.References() {
		result := fetchIcon(ctx, sanitizeName(ref))
		if result.OK() {
			storeIconCache(r.Key(), result)
			return result
		}
	}
	if r, ok := r.(httpRoute); ok {
		// fallback to parse html
		return findIconSlow(ctx, r, uri, nil)
	}
	return &FetchResult{StatusCode: http.StatusNotFound, ErrMsg: "no icon found"}
}

func findIconSlow(ctx context.Context, r httpRoute, uri string, stack []string) *FetchResult {
	select {
	case <-ctx.Done():
		return &FetchResult{StatusCode: http.StatusBadGateway, ErrMsg: "request timeout"}
	default:
	}

	if len(stack) > maxRedirectDepth {
		return &FetchResult{StatusCode: http.StatusBadGateway, ErrMsg: "too many redirects"}
	}

	ctx, cancel := context.WithTimeoutCause(ctx, faviconFetchTimeout, errors.New("favicon request timeout"))
	defer cancel()

	newReq, err := http.NewRequestWithContext(ctx, http.MethodGet, r.TargetURL().String(), nil)
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
				loc = strutils.SanitizeURI(loc)
				if loc == "/" || loc == newReq.URL.Path || slices.Contains(stack, loc) {
					return &FetchResult{StatusCode: http.StatusBadGateway, ErrMsg: "circular redirect"}
				}
				// append current path to stack
				// handles redirect to the same path with different query
				return findIconSlow(ctx, r, loc, append(stack, newReq.URL.Path))
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
		return fetchIconAbsolute(ctx, href)
	default:
		return findIconSlow(ctx, r, href, append(stack, newReq.URL.Path))
	}
}
