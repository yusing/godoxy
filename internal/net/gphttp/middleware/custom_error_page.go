package middleware

import (
	"bytes"
	"io"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/yusing/go-proxy/internal/logging"
	gphttp "github.com/yusing/go-proxy/internal/net/gphttp"
	"github.com/yusing/go-proxy/internal/net/gphttp/httpheaders"
	"github.com/yusing/go-proxy/internal/net/gphttp/middleware/errorpage"
)

type customErrorPage struct{}

var CustomErrorPage = NewMiddleware[customErrorPage]()

const StaticFilePathPrefix = "/$gperrorpage/"

// before implements RequestModifier.
func (customErrorPage) before(w http.ResponseWriter, r *http.Request) (proceed bool) {
	return !ServeStaticErrorPageFile(w, r)
}

// modifyResponse implements ResponseModifier.
func (customErrorPage) modifyResponse(resp *http.Response) error {
	// only handles non-success status code and html/plain content type
	contentType := gphttp.GetContentType(resp.Header)
	if !gphttp.IsSuccess(resp.StatusCode) && (contentType.IsHTML() || contentType.IsPlainText()) {
		errorPage, ok := errorpage.GetErrorPageByStatus(resp.StatusCode)
		if ok {
			logging.Debug().Msgf("error page for status %d loaded", resp.StatusCode)
			_, _ = io.Copy(io.Discard, resp.Body) // drain the original body
			resp.Body.Close()
			resp.Body = io.NopCloser(bytes.NewReader(errorPage))
			resp.ContentLength = int64(len(errorPage))
			resp.Header.Set(httpheaders.HeaderContentLength, strconv.Itoa(len(errorPage)))
			resp.Header.Set(httpheaders.HeaderContentType, "text/html; charset=utf-8")
		} else {
			logging.Error().Msgf("unable to load error page for status %d", resp.StatusCode)
		}
		return nil
	}
	return nil
}

func ServeStaticErrorPageFile(w http.ResponseWriter, r *http.Request) (served bool) {
	path := r.URL.Path
	if path != "" && path[0] != '/' {
		path = "/" + path
	}
	if strings.HasPrefix(path, StaticFilePathPrefix) {
		filename := path[len(StaticFilePathPrefix):]
		file, ok := errorpage.GetStaticFile(filename)
		if !ok {
			logging.Error().Msg("unable to load resource " + filename)
			return false
		}
		ext := filepath.Ext(filename)
		switch ext {
		case ".html":
			w.Header().Set(httpheaders.HeaderContentType, "text/html; charset=utf-8")
		case ".js":
			w.Header().Set(httpheaders.HeaderContentType, "application/javascript; charset=utf-8")
		case ".css":
			w.Header().Set(httpheaders.HeaderContentType, "text/css; charset=utf-8")
		default:
			logging.Error().Msgf("unexpected file type %q for %s", ext, filename)
		}
		if _, err := w.Write(file); err != nil {
			logging.Err(err).Msg("unable to write resource " + filename)
			http.Error(w, "Error page failure", http.StatusInternalServerError)
		}
		return true
	}
	return false
}
