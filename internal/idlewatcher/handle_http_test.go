package idlewatcher

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	idlewatchertypes "github.com/yusing/godoxy/internal/idlewatcher/types"
	"github.com/yusing/godoxy/internal/types"
	gevents "github.com/yusing/goutils/events"
	"github.com/yusing/goutils/http/reverseproxy"
)

func TestWriteLoadingPageDisablesCaching(t *testing.T) {
	w := newTestWatcher(t)
	rec := httptest.NewRecorder()

	err := w.writeLoadingPage(rec)

	require.NoError(t, err)
	require.Equal(t, "no-store, no-cache, must-revalidate, max-age=0", rec.Header().Get("Cache-Control"))
	require.Equal(t, "no-cache", rec.Header().Get("Pragma"))
	require.Equal(t, "0", rec.Header().Get("Expires"))
}

func TestServeHTTPLoadingEndpointsDisableCaching(t *testing.T) {
	for _, path := range []string{
		idlewatchertypes.LoadingPageCSSPath,
		idlewatchertypes.LoadingPageJSPath,
		idlewatchertypes.WakeEventsPath,
	} {
		t.Run(path, func(t *testing.T) {
			w := newTestWatcher(t)
			rec := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodGet, "http://example.com"+path, nil)
			if path == idlewatchertypes.WakeEventsPath {
				ctx, cancel := context.WithCancel(req.Context())
				cancel()
				req = req.WithContext(ctx)
			}

			w.ServeHTTP(rec, req)

			require.Equal(t, "no-store, no-cache, must-revalidate, max-age=0", rec.Header().Get("Cache-Control"))
			require.Equal(t, "no-cache", rec.Header().Get("Pragma"))
			require.Equal(t, "0", rec.Header().Get("Expires"))
		})
	}
}

func TestServeHTTPReadyProxyPreservesUpstreamCacheHeaders(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, _ *http.Request) {
		rw.Header().Set("Cache-Control", "public, max-age=600")
		rw.Header().Set("Expires", "Tue, 21 Apr 2026 00:00:00 GMT")
		rw.WriteHeader(http.StatusOK)
		_, _ = rw.Write([]byte("ok"))
	}))
	defer upstream.Close()

	targetURL, err := url.Parse(upstream.URL)
	require.NoError(t, err)

	w := newTestWatcher(t)
	w.rp = reverseproxy.NewReverseProxy("idlewatcher-test", targetURL, upstream.Client().Transport)
	w.state.Store(&containerState{
		status: idlewatchertypes.ContainerStatusRunning,
		ready:  true,
	})

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "http://example.com/", nil)

	w.ServeHTTP(rec, req)

	require.Equal(t, "public, max-age=600", rec.Header().Get("Cache-Control"))
	require.Equal(t, "Tue, 21 Apr 2026 00:00:00 GMT", rec.Header().Get("Expires"))
	require.Equal(t, http.StatusOK, rec.Code)
	require.Equal(t, "ok", rec.Body.String())
}

func newTestWatcher(t *testing.T) *Watcher {
	t.Helper()

	ticker := time.NewTicker(time.Hour)
	t.Cleanup(ticker.Stop)

	w := &Watcher{
		cfg: &types.IdlewatcherConfig{
			IdlewatcherProviderConfig: types.IdlewatcherProviderConfig{
				Docker: &types.DockerConfig{
					ContainerName: "test-container",
				},
			},
			IdlewatcherConfigBase: types.IdlewatcherConfigBase{
				IdleTimeout: time.Hour,
				WakeTimeout: time.Second,
			},
		},
		idleTicker:    ticker,
		readyNotifyCh: make(chan struct{}, 1),
		events:        gevents.NewHistory(),
	}
	w.lastReset.Store(time.Now())
	w.state.Store(&containerState{
		status: idlewatchertypes.ContainerStatusStopped,
	})
	return w
}
