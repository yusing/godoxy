package idlewatcher

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strconv"

	"github.com/rs/zerolog/log"
	"github.com/yusing/godoxy/internal/homepage/icons"
	iconfetch "github.com/yusing/godoxy/internal/homepage/icons/fetch"
	idlewatcher "github.com/yusing/godoxy/internal/idlewatcher/types"
	httputils "github.com/yusing/goutils/http"

	_ "unsafe"
)

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
		w.rp.ServeHTTP(rw, r)
	}
}

func (w *Watcher) handleWakeEventsSSE(rw http.ResponseWriter, r *http.Request) {
	// Set SSE headers
	rw.Header().Set("Content-Type", "text/event-stream")
	setNoStoreHeaders(rw.Header())
	rw.Header().Set("Connection", "keep-alive")
	rw.Header().Set("Access-Control-Allow-Origin", "*")
	rw.Header().Set("Access-Control-Allow-Headers", "Cache-Control")

	controller := http.NewResponseController(rw)
	ctx := r.Context()

	current, ch, cancel := w.events.SnapshotAndListen()
	defer cancel()

	// Send historical events first
	for _, evt := range current {
		select {
		case <-ctx.Done():
			return
		default:
			err := errors.Join(writeSSE(rw, evt), controller.Flush())
			if err != nil {
				log.Err(err).Msg("Failed to write SSE event")
				return
			}
		}
	}

	// Listen for new events and send them to client
	for {
		select {
		case evt := <-ch:
			err := errors.Join(writeSSE(rw, evt), controller.Flush())
			if err != nil {
				log.Err(err).Msg("Failed to write SSE event")
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
		setNoStoreHeaders(rw.Header())
		serveStaticContent(rw, result.StatusCode, result.ContentType(), result.Icon)
		return false
	case idlewatcher.LoadingPageCSSPath:
		setNoStoreHeaders(rw.Header())
		serveStaticContent(rw, http.StatusOK, "text/css", cssBytes)
		return false
	case idlewatcher.LoadingPageJSPath:
		setNoStoreHeaders(rw.Header())
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
		log.Err(err).Msg("Failed to wake container")
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
	setNoStoreHeaders(rw.Header())
	rw.Header().Add("Connection", "close")
	_ = w.writeLoadingPage(rw)
	return false
}
