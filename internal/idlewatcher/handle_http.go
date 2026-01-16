package idlewatcher

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strconv"

	"github.com/yusing/godoxy/internal/homepage/icons"
	iconfetch "github.com/yusing/godoxy/internal/homepage/icons/fetch"
	idlewatcher "github.com/yusing/godoxy/internal/idlewatcher/types"
	gperr "github.com/yusing/goutils/errs"
	httputils "github.com/yusing/goutils/http"

	_ "unsafe"
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

func (w *Watcher) handleWakeEventsSSE(rw http.ResponseWriter, r *http.Request) {
	// Create a dedicated channel for this SSE connection and register it
	eventCh := make(chan *WakeEvent, 10)
	w.eventChs.Store(eventCh, struct{}{})
	// Clean up when done
	defer func() {
		w.eventChs.Delete(eventCh)
		close(eventCh)
	}()

	// Set SSE headers
	rw.Header().Set("Content-Type", "text/event-stream")
	rw.Header().Set("Cache-Control", "no-cache")
	rw.Header().Set("Connection", "keep-alive")
	rw.Header().Set("Access-Control-Allow-Origin", "*")
	rw.Header().Set("Access-Control-Allow-Headers", "Cache-Control")

	controller := http.NewResponseController(rw)
	ctx := r.Context()

	// Send historical events first
	w.eventHistoryMu.RLock()
	historicalEvents := make([]WakeEvent, len(w.eventHistory))
	copy(historicalEvents, w.eventHistory)
	w.eventHistoryMu.RUnlock()

	for _, event := range historicalEvents {
		select {
		case <-ctx.Done():
			return
		default:
			err := errors.Join(event.WriteSSE(rw), controller.Flush())
			if err != nil {
				gperr.LogError("Failed to write SSE event", err, &w.l)
				return
			}
		}
	}

	// Listen for new events and send them to client
	for {
		select {
		case event := <-eventCh:
			err := errors.Join(event.WriteSSE(rw), controller.Flush())
			if err != nil {
				gperr.LogError("Failed to write SSE event", err, &w.l)
				return
			}
		case <-ctx.Done():
			return
		}
	}
}

func (w *Watcher) getFavIcon(ctx context.Context) (result iconfetch.Result, err error) {
	r := w.route
	hp := r.HomepageItem()
	if hp.Icon != nil {
		if hp.Icon.Source == icons.SourceRelative {
			result, err = iconfetch.FindIcon(ctx, r, *hp.Icon.FullURL, icons.VariantNone)
		} else {
			result, err = iconfetch.FetchFavIconFromURL(ctx, hp.Icon)
		}
	} else {
		// try extract from "link[rel=icon]"
		result, err = iconfetch.FindIcon(ctx, r, "/", icons.VariantNone)
	}
	if result.StatusCode == 0 {
		result.StatusCode = http.StatusOK
	}
	return result, err
}

func serveStaticContent(rw http.ResponseWriter, status int, contentType string, content []byte) {
	rw.Header().Set("Content-Type", contentType)
	rw.Header().Set("Content-Length", strconv.Itoa(len(content)))
	rw.WriteHeader(status)
	rw.Write(content)
}

func (w *Watcher) wakeFromHTTP(rw http.ResponseWriter, r *http.Request) (shouldNext bool) {
	w.resetIdleTimer()

	// handle static files
	switch r.URL.Path {
	case idlewatcher.FavIconPath:
		result, err := w.getFavIcon(r.Context())
		if err != nil {
			rw.WriteHeader(result.StatusCode)
			fmt.Fprint(rw, err)
			return false
		}
		serveStaticContent(rw, result.StatusCode, result.ContentType(), result.Icon)
		return false
	case idlewatcher.LoadingPageCSSPath:
		serveStaticContent(rw, http.StatusOK, "text/css", cssBytes)
		return false
	case idlewatcher.LoadingPageJSPath:
		serveStaticContent(rw, http.StatusOK, "application/javascript", jsBytes)
		return false
	case idlewatcher.WakeEventsPath:
		w.handleWakeEventsSSE(rw, r)
		return false
	}

	// Allow request to proceed if the container is already ready.
	// This check occurs after serving static files because a container can become ready quickly;
	// otherwise, requests for assets may get a 404, leaving the user stuck on the loading screen.
	if w.ready() {
		return true
	}

	// Check if start endpoint is configured and request path matches
	if w.cfg.StartEndpoint != "" && r.URL.Path != w.cfg.StartEndpoint {
		http.Error(rw, "Forbidden: Container can only be started via configured start endpoint", http.StatusForbidden)
		return false
	}

	accept := httputils.GetAccept(r.Header)
	acceptHTML := (r.Method == http.MethodGet && accept.AcceptHTML() || r.RequestURI == "/" && accept.IsEmpty())

	err := w.Wake(r.Context())
	if err != nil {
		gperr.LogError("Failed to wake container", err, &w.l)
		if !acceptHTML {
			http.Error(rw, "Failed to wake container", http.StatusInternalServerError)
			return false
		}
	}

	if !acceptHTML || w.cfg.NoLoadingPage {
		// send a continue response to prevent client wait-header timeout
		rw.WriteHeader(http.StatusContinue)
		ready := w.waitForReady(r.Context())
		if !ready {
			serveStaticContent(rw, http.StatusInternalServerError, "text/plain", []byte("Timeout waiting for container to become ready"))
			return false
		}
		return true
	}

	// Send a loading response to the client
	rw.Header().Set("Content-Type", "text/html; charset=utf-8")
	rw.Header().Set("Cache-Control", "no-cache")
	rw.Header().Add("Cache-Control", "no-store")
	rw.Header().Add("Cache-Control", "must-revalidate")
	rw.Header().Add("Connection", "close")
	_ = w.writeLoadingPage(rw)
	return false
}
