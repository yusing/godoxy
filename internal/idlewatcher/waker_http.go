package idlewatcher

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/yusing/go-proxy/internal/api/v1/favicon"
	gphttp "github.com/yusing/go-proxy/internal/net/gphttp"
	"github.com/yusing/go-proxy/internal/net/gphttp/httpheaders"
)

type ForceCacheControl struct {
	expires string
	http.ResponseWriter
}

func (f *ForceCacheControl) WriteHeader(code int) {
	f.ResponseWriter.Header().Set("Cache-Control", "must-revalidate")
	f.ResponseWriter.Header().Set("Expires", f.expires)
	f.ResponseWriter.WriteHeader(code)
}

func (f *ForceCacheControl) Unwrap() http.ResponseWriter {
	return f.ResponseWriter
}

// ServeHTTP implements http.Handler.
func (w *Watcher) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	shouldNext := w.wakeFromHTTP(rw, r)
	if !shouldNext {
		return
	}
	select {
	case <-r.Context().Done():
		return
	default:
		f := &ForceCacheControl{expires: w.expires().Format(http.TimeFormat), ResponseWriter: rw}
		w.rp.ServeHTTP(f, r)
	}
}

func (w *Watcher) cancelled(reqCtx context.Context, rw http.ResponseWriter) bool {
	select {
	case <-reqCtx.Done():
		w.WakeDebug().Str("cause", context.Cause(reqCtx).Error()).Msg("canceled")
		return true
	case <-w.task.Context().Done():
		w.WakeDebug().Str("cause", w.task.FinishCause().Error()).Msg("canceled")
		http.Error(rw, "Service unavailable", http.StatusServiceUnavailable)
		return true
	default:
		return false
	}
}

func isFaviconPath(path string) bool {
	return path == "/favicon.ico"
}

func (w *Watcher) wakeFromHTTP(rw http.ResponseWriter, r *http.Request) (shouldNext bool) {
	w.resetIdleTimer()

	// pass through if container is already ready
	if w.ready() {
		return true
	}

	// handle favicon request
	if isFaviconPath(r.URL.Path) {
		r.URL.RawQuery = "alias=" + w.route.TargetName()
		favicon.GetFavIcon(rw, r)
		return false
	}

	// Check if start endpoint is configured and request path matches
	if w.Config().StartEndpoint != "" && r.URL.Path != w.Config().StartEndpoint {
		http.Error(rw, "Forbidden: Container can only be started via configured start endpoint", http.StatusForbidden)
		return false
	}

	accept := gphttp.GetAccept(r.Header)
	acceptHTML := (r.Method == http.MethodGet && accept.AcceptHTML() || r.RequestURI == "/" && accept.IsEmpty())

	isCheckRedirect := r.Header.Get(httpheaders.HeaderGoDoxyCheckRedirect) != ""
	if !isCheckRedirect && acceptHTML {
		// Send a loading response to the client
		body := w.makeLoadingPageBody()
		rw.Header().Set("Content-Type", "text/html; charset=utf-8")
		rw.Header().Set("Content-Length", strconv.Itoa(len(body)))
		rw.Header().Set("Cache-Control", "no-cache")
		rw.Header().Add("Cache-Control", "no-store")
		rw.Header().Add("Cache-Control", "must-revalidate")
		rw.Header().Add("Connection", "close")
		if _, err := rw.Write(body); err != nil {
			w.Err(err).Msg("error writing http response")
		}
		return false
	}

	ctx, cancel := context.WithTimeoutCause(r.Context(), w.Config().WakeTimeout, errors.New("wake timeout"))
	defer cancel()

	if w.cancelled(ctx, rw) {
		return false
	}

	w.WakeTrace().Msg("signal received")
	err := w.wakeIfStopped()
	if err != nil {
		w.WakeError(err)
		http.Error(rw, "Error waking container", http.StatusInternalServerError)
		return false
	}

	for {
		if w.cancelled(ctx, rw) {
			return false
		}

		ready, err := w.checkUpdateState()
		if err != nil {
			http.Error(rw, "Error waking container", http.StatusInternalServerError)
			return false
		}
		if ready {
			w.resetIdleTimer()
			if isCheckRedirect {
				w.Debug().Msgf("redirecting to %s ...", w.hc.URL())
				rw.WriteHeader(http.StatusOK)
				return false
			}
			w.Debug().Msgf("passing through to %s ...", w.hc.URL())
			return true
		}

		// retry until the container is ready or timeout
		time.Sleep(idleWakerCheckInterval)
	}
}
