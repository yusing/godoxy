package homepage

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"
	"github.com/gin-gonic/gin"
	"github.com/vincent-petithory/dataurl"
	apitypes "github.com/yusing/godoxy/internal/api/types"
	gphttp "github.com/yusing/godoxy/internal/net/gphttp"
	"github.com/yusing/goutils/cache"
	httputils "github.com/yusing/goutils/http"
	strutils "github.com/yusing/goutils/strings"
)

type FetchResult struct {
	Icon       []byte
	StatusCode int

	contentType string
}

func FetchResultWithErrorf(statusCode int, msgFmt string, args ...any) (FetchResult, error) {
	return FetchResult{StatusCode: statusCode}, fmt.Errorf(msgFmt, args...)
}

func FetchResultOK(icon []byte, contentType string) (FetchResult, error) {
	return FetchResult{Icon: icon, contentType: contentType}, nil
}

func GinFetchError(c *gin.Context, statusCode int, err error) {
	if statusCode == 0 {
		statusCode = http.StatusInternalServerError
	}
	if statusCode == http.StatusInternalServerError {
		c.Error(apitypes.InternalServerError(err, "unexpected error"))
	} else {
		c.JSON(statusCode, apitypes.Error(err.Error()))
	}
}

const faviconFetchTimeout = 3 * time.Second

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

func FetchFavIconFromURL(ctx context.Context, iconURL *IconURL) (FetchResult, error) {
	switch iconURL.IconSource {
	case IconSourceAbsolute:
		return FetchIconAbsolute(ctx, iconURL.URL())
	case IconSourceRelative:
		return FetchResultWithErrorf(http.StatusBadRequest, "unexpected relative icon")
	case IconSourceWalkXCode, IconSourceSelfhSt:
		return fetchKnownIcon(ctx, iconURL)
	}
	return FetchResultWithErrorf(http.StatusBadRequest, "invalid icon source")
}

var FetchIconAbsolute = cache.NewKeyFunc(func(ctx context.Context, url string) (FetchResult, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return FetchResultWithErrorf(http.StatusInternalServerError, "cannot create request: %w", err)
	}

	resp, err := gphttp.Do(req)
	if err == nil {
		defer resp.Body.Close()
	} else {
		if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
			return FetchResultWithErrorf(http.StatusBadGateway, "request timeout")
		}
		return FetchResultWithErrorf(http.StatusBadGateway, "connection error: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return FetchResultWithErrorf(resp.StatusCode, "upstream error: http %d", resp.StatusCode)
	}

	icon, err := io.ReadAll(resp.Body)
	if err != nil {
		return FetchResultWithErrorf(http.StatusInternalServerError, "failed to read response body: %w", err)
	}

	if len(icon) == 0 {
		return FetchResultWithErrorf(http.StatusNotFound, "empty icon")
	}

	res := FetchResult{Icon: icon}
	if contentType := resp.Header.Get("Content-Type"); contentType != "" {
		res.contentType = contentType
	}
	// else leave it empty
	return res, nil
}).WithMaxEntries(200).WithRetriesExponentialBackoff(3).WithTTL(4 * time.Hour).Build()

var nameSanitizer = strings.NewReplacer(
	"_", "-",
	" ", "-",
	"(", "",
	")", "",
)

func sanitizeName(name string) string {
	return strings.ToLower(nameSanitizer.Replace(name))
}

func fetchKnownIcon(ctx context.Context, url *IconURL) (FetchResult, error) {
	// if icon isn't in the list, no need to fetch
	if !url.HasIcon() {
		return FetchResult{StatusCode: http.StatusNotFound}, errors.New("no such icon")
	}

	return FetchIconAbsolute(ctx, url.URL())
}

func fetchIcon(ctx context.Context, filename string) (FetchResult, error) {
	for _, fileType := range []string{"svg", "webp", "png"} {
		result, err := fetchKnownIcon(ctx, NewSelfhStIconURL(filename, fileType))
		if err == nil {
			return result, err
		}
		result, err = fetchKnownIcon(ctx, NewWalkXCodeIconURL(filename, fileType))
		if err == nil {
			return result, err
		}
	}
	return FetchResultWithErrorf(http.StatusNotFound, "no icon found")
}

func FindIcon(ctx context.Context, r route, uri string) (FetchResult, error) {
	for _, ref := range r.References() {
		result, err := fetchIcon(ctx, sanitizeName(ref))
		if err == nil {
			return result, err
		}
	}
	if r, ok := r.(httpRoute); ok {
		// fallback to parse html
		return findIconSlowCached(context.WithValue(ctx, "route", r), uri)
	}
	return FetchResultWithErrorf(http.StatusNotFound, "no icon found")
}

var findIconSlowCached = cache.NewKeyFunc(func(ctx context.Context, key string) (FetchResult, error) {
	r := ctx.Value("route").(httpRoute)
	return findIconSlow(ctx, r, key, nil)
}).WithMaxEntries(200).Build() // no retries, no ttl

func findIconSlow(ctx context.Context, r httpRoute, uri string, stack []string) (FetchResult, error) {
	select {
	case <-ctx.Done():
		return FetchResultWithErrorf(http.StatusBadGateway, "request timeout")
	default:
	}

	if len(stack) > maxRedirectDepth {
		return FetchResultWithErrorf(http.StatusBadGateway, "too many redirects")
	}

	ctx, cancel := context.WithTimeoutCause(ctx, faviconFetchTimeout, errors.New("favicon request timeout"))
	defer cancel()

	newReq, err := http.NewRequestWithContext(ctx, http.MethodGet, r.TargetURL().String(), nil)
	if err != nil {
		return FetchResultWithErrorf(http.StatusInternalServerError, "cannot create request: %w", err)
	}
	newReq.Header.Set("Accept-Encoding", "identity") // disable compression

	u, err := url.ParseRequestURI(strutils.SanitizeURI(uri))
	if err != nil {
		return FetchResultWithErrorf(http.StatusInternalServerError, "cannot parse uri: %w", err)
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
			return FetchResultWithErrorf(http.StatusBadGateway, "connection error")
		default:
			if loc := c.Header().Get("Location"); loc != "" {
				loc = strutils.SanitizeURI(loc)
				if loc == "/" || loc == newReq.URL.Path || slices.Contains(stack, loc) {
					return FetchResultWithErrorf(http.StatusBadGateway, "circular redirect")
				}
				// append current path to stack
				// handles redirect to the same path with different query
				return findIconSlow(ctx, r, loc, append(stack, newReq.URL.Path))
			}
		}
		return FetchResultWithErrorf(c.status, "upstream error: status %d, %s", c.status, c.data)
	}
	// return icon data
	if !httputils.GetContentType(c.header).IsHTML() {
		return FetchResultOK(c.data, c.header.Get("Content-Type"))
	}
	// try extract from "link[rel=icon]" from path "/"
	doc, err := goquery.NewDocumentFromReader(bytes.NewBuffer(c.data))
	if err != nil {
		return FetchResultWithErrorf(http.StatusInternalServerError, "failed to parse html: %w", err)
	}
	ele := doc.Find("head > link[rel=icon]").First()
	if ele.Length() == 0 {
		return FetchResultWithErrorf(http.StatusNotFound, "icon element not found")
	}
	href := ele.AttrOr("href", "")
	if href == "" {
		return FetchResultWithErrorf(http.StatusNotFound, "icon href not found")
	}
	// https://en.wikipedia.org/wiki/Data_URI_scheme
	if strings.HasPrefix(href, "data:image/") {
		dataURI, err := dataurl.DecodeString(href)
		if err != nil {
			return FetchResultWithErrorf(http.StatusInternalServerError, "failed to decode favicon: %w", err)
		}
		return FetchResultOK(dataURI.Data, dataURI.ContentType())
	}
	switch {
	case strings.HasPrefix(href, "http://"), strings.HasPrefix(href, "https://"):
		return FetchIconAbsolute(ctx, href)
	default:
		return findIconSlow(ctx, r, href, append(stack, newReq.URL.Path))
	}
}
