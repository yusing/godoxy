package idlewatcher

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/yusing/godoxy/internal/health"
	idlewatchertypes "github.com/yusing/godoxy/internal/idlewatcher/runtime"
	watcherEvents "github.com/yusing/godoxy/internal/watcher/events"
	gevents "github.com/yusing/goutils/events"
	"github.com/yusing/goutils/http/reverseproxy"
	"github.com/yusing/goutils/task"
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

func TestServeHTTPReadyRequestBypassesWakeSingleflight(t *testing.T) {
	w := newTestWatcher(t)
	w.setReady()
	operationStarted := make(chan struct{})
	releaseOperation := make(chan struct{})
	release := sync.OnceFunc(func() { close(releaseOperation) })
	t.Cleanup(release)
	sharedResult := singleFlight.DoChan(w.Key(), func() (any, error) {
		close(operationStarted)
		<-releaseOperation
		return nil, nil
	})
	requireChannelClosed(t, operationStarted, "shared wake operation did not start")

	result := make(chan bool, 1)
	go func() {
		result <- w.wakeFromHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "http://example.com/", nil))
	}()
	select {
	case shouldNext := <-result:
		require.True(t, shouldNext)
	case <-time.After(time.Second):
		t.Fatal("ready HTTP request waited for active wake singleflight")
	}

	release()
	require.NoError(t, (<-sharedResult).Err)
}

func TestServeHTTPLoadingPageDoesNotWaitForWake(t *testing.T) {
	w, provider := newBlockingWakeWatcher(t)
	rec := httptest.NewRecorder()
	reqCtx, cancelRequest := context.WithCancel(t.Context())
	req := httptest.NewRequest(http.MethodGet, "http://example.com/", nil).WithContext(reqCtx)

	returned := make(chan struct{})
	go func() {
		w.ServeHTTP(rec, req)
		close(returned)
	}()

	wakeCtx := receiveWakeContext(t, provider.started)
	requireChannelClosed(t, returned, "loading page handler did not return while provider startup was blocked")
	require.Equal(t, http.StatusOK, rec.Code)
	require.Contains(t, rec.Body.String(), "loading-dots")

	// The navigation request ends when the loading page is returned, but its wake operation
	// must continue so the page's subsequent SSE request can observe startup progress.
	cancelRequest()
	select {
	case <-wakeCtx.Done():
		t.Fatal("loading page wake inherited the completed request context")
	default:
	}

	secondReturned := make(chan struct{})
	go func() {
		w.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "http://example.com/", nil))
		close(secondReturned)
	}()
	requireChannelClosed(t, secondReturned, "duplicate loading page handler did not return")
	require.EqualValues(t, 1, provider.starts.Load(), "concurrent loading requests must share one wake operation")

	close(provider.release)
}

func TestServeHTTPLoadingPageRequestClassification(t *testing.T) {
	tests := []struct {
		name          string
		path          string
		accept        string
		startEndpoint string
		wantWake      bool
		wantStatus    int
		wantType      string
	}{
		{
			name:       "html navigation",
			path:       "/app",
			accept:     "text/html",
			wantWake:   true,
			wantStatus: http.StatusOK,
			wantType:   "text/html; charset=utf-8",
		},
		{
			name:       "malformed accept falls back to navigation",
			path:       "/",
			accept:     "not a media type",
			wantWake:   true,
			wantStatus: http.StatusOK,
			wantType:   "text/html; charset=utf-8",
		},
		{
			name:       "reserved path suffix is not an asset collision",
			path:       idlewatchertypes.LoadingPageJSPath + ".map",
			accept:     "text/html",
			wantWake:   true,
			wantStatus: http.StatusOK,
			wantType:   "text/html; charset=utf-8",
		},
		{
			name:       "unknown future internal path remains a navigation",
			path:       idlewatchertypes.PathPrefix + "future-endpoint",
			accept:     "text/html",
			wantWake:   true,
			wantStatus: http.StatusOK,
			wantType:   "text/html; charset=utf-8",
		},
		{
			name:       "exact static asset does not wake",
			path:       idlewatchertypes.LoadingPageJSPath,
			accept:     "text/html",
			wantWake:   false,
			wantStatus: http.StatusOK,
			wantType:   "application/javascript",
		},
		{
			name:          "start endpoint mismatch rejects without waking",
			path:          "/other",
			accept:        "text/html",
			startEndpoint: "/start",
			wantWake:      false,
			wantStatus:    http.StatusForbidden,
			wantType:      "text/plain; charset=utf-8",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w, provider := newBlockingWakeWatcher(t)
			close(provider.release)
			w.cfg.StartEndpoint = tt.startEndpoint
			rec := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodGet, "http://example.com"+tt.path, nil)
			req.Header.Set("Accept", tt.accept)

			w.ServeHTTP(rec, req)

			require.Equal(t, tt.wantStatus, rec.Code)
			require.Equal(t, tt.wantType, rec.Header().Get("Content-Type"))
			if tt.wantWake {
				receiveWakeContext(t, provider.started)
				require.EqualValues(t, 1, provider.starts.Load())
				return
			}
			select {
			case <-provider.started:
				t.Fatal("request unexpectedly started the container")
			default:
			}
		})
	}
}

func TestServeHTTPNonHTMLRequestStillWaitsForWake(t *testing.T) {
	w, provider := newBlockingWakeWatcher(t)
	rec := httptest.NewRecorder()
	reqCtx, cancelRequest := context.WithCancel(t.Context())
	req := httptest.NewRequest(http.MethodPost, "http://example.com/", nil).WithContext(reqCtx)
	returned := make(chan struct{})
	go func() {
		w.ServeHTTP(rec, req)
		close(returned)
	}()

	wakeCtx := receiveWakeContext(t, provider.started)
	select {
	case <-returned:
		t.Fatal("non-HTML request returned before synchronous wake completed")
	default:
	}

	cancelRequest()
	select {
	case <-returned:
		t.Fatal("non-HTML request cancellation interrupted the synchronous wake")
	case <-wakeCtx.Done():
		t.Fatal("non-HTML request cancellation canceled the wake operation")
	default:
	}
	close(provider.release)
	requireChannelClosed(t, returned, "non-HTML request did not return after wake completed")
	require.Equal(t, http.StatusContinue, rec.Code)
}

func TestServeHTTPNonHTMLCancellationDoesNotCancelBackgroundWake(t *testing.T) {
	w, provider := newBlockingWakeWatcher(t)
	w.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "http://example.com/", nil))
	wakeCtx := receiveWakeContext(t, provider.started)

	rec := httptest.NewRecorder()
	reqCtx, cancelRequest := context.WithCancel(t.Context())
	req := httptest.NewRequest(http.MethodPost, "http://example.com/", nil).WithContext(reqCtx)
	returned := make(chan struct{})
	go func() {
		w.ServeHTTP(rec, req)
		close(returned)
	}()

	cancelRequest()
	select {
	case <-returned:
		t.Fatal("canceled non-HTML request returned before the shared wake completed")
	case <-wakeCtx.Done():
		t.Fatal("canceling a joined request canceled the shared background wake")
	default:
	}
	close(provider.release)
	requireChannelClosed(t, returned, "non-HTML request did not return after the shared wake completed")
	require.Equal(t, http.StatusContinue, rec.Code)
}

func TestServeHTTPLoadingPageWakeSurvivesInitiatingNonHTMLCancellation(t *testing.T) {
	w, provider := newBlockingWakeWatcher(t)
	rec := httptest.NewRecorder()
	reqCtx, cancelRequest := context.WithCancel(t.Context())
	req := httptest.NewRequest(http.MethodPost, "http://example.com/", nil).WithContext(reqCtx)
	returned := make(chan struct{})
	go func() {
		w.ServeHTTP(rec, req)
		close(returned)
	}()

	wakeCtx := receiveWakeContext(t, provider.started)
	loadingRec := httptest.NewRecorder()
	w.ServeHTTP(loadingRec, httptest.NewRequest(http.MethodGet, "http://example.com/", nil))
	require.Equal(t, http.StatusOK, loadingRec.Code)
	require.Contains(t, loadingRec.Body.String(), "loading-dots")

	cancelRequest()
	select {
	case <-returned:
		t.Fatal("initiating non-HTML request cancellation interrupted the shared wake")
	case <-wakeCtx.Done():
		t.Fatal("initiating request cancellation canceled the shared loading-page wake")
	default:
	}
	close(provider.release)
	requireChannelClosed(t, returned, "initiating non-HTML request did not return after wake completed")
	require.Equal(t, http.StatusContinue, rec.Code)
}

func TestServeHTTPLoadingPageWakeFailureIsPublished(t *testing.T) {
	w, provider := newBlockingWakeWatcher(t)
	provider.startErr = errors.New("provider start failed")
	close(provider.release)
	_, eventCh, stopListening := w.events.SnapshotAndListen()
	defer stopListening()

	w.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "http://example.com/", nil))
	receiveWakeContext(t, provider.started)

	timer := time.NewTimer(time.Second)
	defer timer.Stop()
	for {
		select {
		case event := <-eventCh:
			if event.Action == string(WakeEventError) {
				wakeEvent, ok := event.Data.(*WakeEvent)
				require.True(t, ok)
				require.Contains(t, wakeEvent.Error, provider.startErr.Error())
				return
			}
		case <-timer.C:
			t.Fatal("wake failure was not published to loading page event listeners")
		}
	}
}

func TestServeHTTPLoadingPageRetriesCachedError(t *testing.T) {
	w, provider := newBlockingWakeWatcher(t)
	staleErr := errors.New("stale startup timeout")
	w.setError(staleErr)

	rec := httptest.NewRecorder()
	w.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "http://example.com/", nil))

	require.Equal(t, http.StatusOK, rec.Code)
	for _, event := range w.events.Get() {
		wakeEvent, ok := event.Data.(*WakeEvent)
		require.True(t, ok)
		require.NotContains(t, wakeEvent.Error, staleErr.Error())
	}

	receiveWakeContext(t, provider.started)
	require.EqualValues(t, 1, provider.statusCalls.Load())
	for _, event := range w.events.Get() {
		wakeEvent, ok := event.Data.(*WakeEvent)
		require.True(t, ok)
		require.NotContains(t, wakeEvent.Error, staleErr.Error())
	}

	close(provider.release)
	require.Eventually(t, func() bool {
		return w.error() == nil && w.wakeInProgress()
	}, time.Second, time.Millisecond, "successful retry did not replace the cached error with starting state")
}

func TestServeHTTPLoadingPageJoinPreservesActiveRetryHistory(t *testing.T) {
	w, provider := newBlockingWakeWatcher(t)
	dep, depProvider := newBlockingWakeWatcher(t)
	w.dependsOn = []*dependency{{Watcher: dep}}
	w.setError(errors.New("stale startup timeout"))
	defer close(provider.release)
	defer close(depProvider.release)

	w.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "http://example.com/", nil))
	receiveWakeContext(t, depProvider.started)
	require.Eventually(t, func() bool {
		return historyContainsAction(w, WakeEventWakingDep)
	}, time.Second, time.Millisecond, "dependency progress was not published")

	w.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "http://example.com/", nil))
	require.True(t, historyContainsAction(w, WakeEventWakingDep),
		"joining loading-page request erased active retry progress")
}

func TestWakeRechecksErroredRunningContainer(t *testing.T) {
	w, provider := newBlockingWakeWatcher(t)
	provider.status = idlewatchertypes.ContainerStatusRunning
	w.setError(errors.New("stale health failure"))

	err := w.Wake(t.Context())

	require.NoError(t, err)
	require.EqualValues(t, 1, provider.statusCalls.Load())
	require.Zero(t, provider.starts.Load())
	require.NoError(t, w.error())
	require.True(t, w.wakeInProgress())
}

func TestWakeDoesNotCacheInvalidProviderStatus(t *testing.T) {
	for _, status := range []idlewatchertypes.ContainerStatus{"", "future"} {
		t.Run(string(status), func(t *testing.T) {
			w, provider := newBlockingWakeWatcher(t)
			provider.status = status
			w.setError(errors.New("retryable failure"))

			for range 2 {
				err := w.Wake(t.Context())
				require.ErrorContains(t, err, "unexpected container status")
			}
			require.EqualValues(t, 2, provider.statusCalls.Load())
		})
	}
}

func TestWakeSingleflightUsesProviderSpecificKey(t *testing.T) {
	first, firstProvider := newBlockingWakeWatcher(t)
	second, secondProvider := newBlockingWakeWatcher(t)
	second.cfg.Docker.ContainerName = first.cfg.Docker.ContainerName
	releaseProviders := sync.OnceFunc(func() {
		close(firstProvider.release)
		close(secondProvider.release)
	})
	defer releaseProviders()

	firstResult := make(chan error, 1)
	secondResult := make(chan error, 1)
	go func() { firstResult <- first.Wake(t.Context()) }()
	go func() { secondResult <- second.Wake(t.Context()) }()

	receiveWakeContext(t, firstProvider.started)
	receiveWakeContext(t, secondProvider.started)
	releaseProviders()
	require.NoError(t, <-firstResult)
	require.NoError(t, <-secondResult)
	require.EqualValues(t, 1, firstProvider.starts.Load())
	require.EqualValues(t, 1, secondProvider.starts.Load())
}

func TestServeHTTPLoadingPageDependencyHealthWaitRetriesWithoutRestart(t *testing.T) {
	w, _ := newBlockingWakeWatcher(t)
	w.cfg.WakeTimeout = 20 * time.Millisecond
	dep, depProvider := newBlockingWakeWatcher(t)
	dep.cfg.WakeTimeout = 20 * time.Millisecond
	close(depProvider.release)
	dep.hc = &unhealthyHealthChecker{targetURL: &url.URL{Scheme: "http", Host: "dependency.test"}}
	w.dependsOn = []*dependency{{Watcher: dep, waitHealthy: true}}
	_, eventCh, stopListening := w.events.SnapshotAndListen()
	defer stopListening()

	w.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "http://example.com/", nil))
	receiveWakeContext(t, depProvider.started)
	waitForWakeError(t, eventCh, "timeout")

	require.Eventually(t, func() bool {
		w.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "http://example.com/", nil))
		select {
		case event := <-eventCh:
			if event.Action != string(WakeEventError) {
				return false
			}
			wakeEvent, ok := event.Data.(*WakeEvent)
			return ok && strings.Contains(wakeEvent.Error, "timeout")
		default:
			return false
		}
	}, time.Second, 10*time.Millisecond, "loading page did not publish a fresh dependency health error")
	require.EqualValues(t, 1, depProvider.starts.Load(), "health retry restarted an already-running dependency")
}

func TestServeHTTPLoadingPageDependencyStartUsesDependencyTimeout(t *testing.T) {
	w, provider := newBlockingWakeWatcher(t)
	w.cfg.WakeTimeout = 20 * time.Millisecond
	dep, depProvider := newBlockingWakeWatcher(t)
	dep.cfg.WakeTimeout = time.Second
	w.dependsOn = []*dependency{{Watcher: dep}}

	w.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "http://example.com/", nil))
	depWakeCtx := receiveWakeContext(t, depProvider.started)
	deadline, ok := depWakeCtx.Deadline()
	require.True(t, ok)
	require.Greater(t, time.Until(deadline), 500*time.Millisecond,
		"dependency start was truncated by the root target timeout")
	require.EqualValues(t, 1, depProvider.starts.Load())
	close(depProvider.release)
	close(provider.release)
}

func TestTotalWakeTimeoutIncludesUniqueRecursiveDependencies(t *testing.T) {
	root := newTestWatcher(t)
	direct := newTestWatcher(t)
	nested := newTestWatcher(t)
	shared := newTestWatcher(t)

	root.cfg.WakeTimeout = time.Second
	direct.cfg.WakeTimeout = 2 * time.Second
	nested.cfg.WakeTimeout = 3 * time.Second
	shared.cfg.WakeTimeout = 4 * time.Second
	for _, watcher := range []*Watcher{root, direct, nested, shared} {
		watcher.cfg.Docker.ContainerID = watcher.cfg.ContainerName()
	}

	nested.dependsOn = []*dependency{{Watcher: shared}}
	direct.dependsOn = []*dependency{{Watcher: nested}}
	root.dependsOn = []*dependency{
		{Watcher: direct},
		{Watcher: shared},
	}

	require.Equal(t, 10*time.Second, root.totalWakeTimeout())
}

func TestDependenciesCacheInvalidatesWithGraphVersion(t *testing.T) {
	root := newTestWatcher(t)
	direct := newTestWatcher(t)
	nested := newTestWatcher(t)
	firstLeaf := newTestWatcher(t)
	secondLeaf := newTestWatcher(t)

	nested.setDependencies([]*dependency{{Watcher: firstLeaf}})
	root.setDependencies([]*dependency{{Watcher: direct}, {Watcher: nested}})
	first := root.dependencies()
	second := root.dependencies()
	require.Len(t, first, 3)
	require.Same(t, first[0], second[0])
	require.True(t, &first[0] == &second[0], "dependency cache rebuilt without a graph change")
	require.Zero(t, testing.AllocsPerRun(100, func() {
		_ = root.dependencies()
	}), "cached dependency lookup allocated")

	nested.setDependencies([]*dependency{{Watcher: secondLeaf}})
	refreshed := root.dependencies()
	require.Len(t, refreshed, 3)
	refreshedKeys := make([]string, 0, len(refreshed))
	for _, dep := range refreshed {
		refreshedKeys = append(refreshedKeys, dep.Key())
	}
	require.ElementsMatch(t, []string{direct.Key(), nested.Key(), secondLeaf.Key()}, refreshedKeys)
}

func TestWakeCallerCancellationDoesNotStopSharedOperation(t *testing.T) {
	w, provider := newBlockingWakeWatcher(t)
	firstCtx, cancelFirst := context.WithCancel(t.Context())
	firstResult := make(chan error, 1)
	go func() {
		firstResult <- w.Wake(firstCtx)
	}()

	wakeCtx := receiveWakeContext(t, provider.started)
	secondCtx := &observedDoneContext{Context: t.Context(), doneObserved: make(chan struct{})}
	secondResult := make(chan error, 1)
	go func() {
		secondResult <- w.Wake(secondCtx)
	}()
	requireChannelClosed(t, secondCtx.doneObserved, "second caller did not join the shared wake")

	cancelFirst()

	select {
	case err := <-firstResult:
		require.ErrorIs(t, err, context.Canceled)
	case <-time.After(time.Second):
		t.Fatal("canceled caller did not stop waiting for the shared wake")
	}
	select {
	case <-wakeCtx.Done():
		t.Fatal("canceling the initiating caller canceled the shared wake operation")
	default:
	}

	close(provider.release)
	select {
	case err := <-secondResult:
		require.NoError(t, err)
	case <-time.After(time.Second):
		t.Fatal("joined caller did not receive the shared wake result")
	}
}

func TestWakeWatcherCancellationStopsOperation(t *testing.T) {
	w, provider := newBlockingWakeWatcher(t)
	result := make(chan error, 1)
	go func() {
		result <- w.Wake(context.Background())
	}()

	wakeCtx := receiveWakeContext(t, provider.started)
	w.task.Finish(errors.New("watcher stopped"))

	requireChannelClosed(t, wakeCtx.Done(), "watcher cancellation did not cancel the provider wake context")
	select {
	case err := <-result:
		require.ErrorContains(t, err, "watcher stopped")
	case <-time.After(time.Second):
		t.Fatal("wake did not return after watcher cancellation")
	}
}

func TestWakeDependencyOutlivesParentCancellation(t *testing.T) {
	root, _ := newBlockingWakeWatcher(t)
	dep, depProvider := newBlockingWakeWatcher(t)
	root.dependsOn = []*dependency{{Watcher: dep}}
	result := make(chan error, 1)
	go func() {
		result <- root.Wake(context.Background())
	}()

	depWakeCtx := receiveWakeContext(t, depProvider.started)
	root.task.Finish(errors.New("parent watcher stopped"))

	select {
	case err := <-result:
		require.ErrorContains(t, err, "parent watcher stopped")
	case <-time.After(time.Second):
		t.Fatal("parent wake did not return after parent watcher cancellation")
	}
	select {
	case <-depWakeCtx.Done():
		t.Fatal("parent watcher cancellation canceled the dependency wake")
	default:
	}

	close(depProvider.release)
	requireChannelClosed(t, depWakeCtx.Done(), "dependency wake did not finish after provider release")
}

func TestWaitForReadyIgnoresStaleNotification(t *testing.T) {
	w := newTestWatcher(t)
	w.setReady()
	w.setError(errors.New("not ready anymore"))
	ctx := &observedDoneContext{Context: t.Context(), doneObserved: make(chan struct{})}
	result := make(chan bool, 1)
	go func() {
		result <- w.waitForReady(ctx)
	}()

	requireChannelClosed(t, ctx.doneObserved, "readiness waiter did not block after consuming stale notification")
	select {
	case <-result:
		t.Fatal("stale readiness notification released waiter")
	default:
	}

	w.setReady()
	select {
	case ready := <-result:
		require.True(t, ready)
	case <-time.After(time.Second):
		t.Fatal("current readiness notification did not release waiter")
	}
}

func TestWaitForReadyBroadcastsReadyToAllWaiters(t *testing.T) {
	w := newTestWatcher(t)
	w.setStarting()

	const waiterCount = 3
	results := make(chan bool, waiterCount)
	for range waiterCount {
		ctx := &observedDoneContext{Context: t.Context(), doneObserved: make(chan struct{})}
		go func() {
			results <- w.waitForReady(ctx)
		}()
		requireChannelClosed(t, ctx.doneObserved, "readiness waiter did not start waiting")
	}

	w.setReady()
	for range waiterCount {
		select {
		case ready := <-results:
			require.True(t, ready)
		case <-time.After(time.Second):
			t.Fatal("ready transition did not release every waiter")
		}
	}
}

func newTestWatcher(t *testing.T) *Watcher {
	t.Helper()

	idleTicker := time.NewTicker(time.Hour)
	healthTicker := time.NewTicker(time.Hour)
	t.Cleanup(idleTicker.Stop)
	t.Cleanup(healthTicker.Stop)
	containerName := fmt.Sprintf("test-container-%d", testWatcherID.Add(1))
	watcherTask := task.GetTestTask(t).Subtask("idlewatcher_http", true)
	t.Cleanup(func() {
		watcherTask.FinishAndWait(nil)
	})

	w := &Watcher{
		cfg: &idlewatchertypes.Config{
			IdlewatcherProviderConfig: idlewatchertypes.ProviderConfig{
				Docker: &idlewatchertypes.DockerConfig{
					ContainerID:   containerName,
					ContainerName: containerName,
				},
			},
			IdlewatcherConfigBase: idlewatchertypes.ConfigBase{
				IdleTimeout: time.Hour,
				WakeTimeout: time.Second,
			},
		},
		idleTicker:     idleTicker,
		healthTicker:   healthTicker,
		stateChangedCh: make(chan struct{}),
		events:         gevents.NewHistory(),
		task:           watcherTask,
	}
	w.lastReset.Store(time.Now())
	w.state.Store(&containerState{
		status: idlewatchertypes.ContainerStatusStopped,
	})
	return w
}

func newBlockingWakeWatcher(t *testing.T) (*Watcher, *blockingStartProvider) {
	t.Helper()
	w := newTestWatcher(t)
	provider := &blockingStartProvider{
		started: make(chan context.Context, 1),
		release: make(chan struct{}),
		status:  idlewatchertypes.ContainerStatusStopped,
	}
	w.provider.Store(provider)
	return w, provider
}

func receiveWakeContext(t *testing.T, started <-chan context.Context) context.Context {
	t.Helper()
	select {
	case ctx := <-started:
		return ctx
	case <-time.After(time.Second):
		t.Fatal("container wake did not start")
		return nil
	}
}

func waitForWakeError(t *testing.T, eventCh <-chan gevents.Event, contains string) {
	t.Helper()
	timer := time.NewTimer(time.Second)
	defer timer.Stop()
	for {
		select {
		case event := <-eventCh:
			if event.Action != string(WakeEventError) {
				continue
			}
			wakeEvent, ok := event.Data.(*WakeEvent)
			require.True(t, ok)
			require.Contains(t, wakeEvent.Error, contains)
			return
		case <-timer.C:
			t.Fatal("wake error was not published to loading page event listeners")
		}
	}
}

func historyContainsAction(w *Watcher, action WakeEventType) bool {
	for _, event := range w.events.Get() {
		if event.Action == string(action) {
			return true
		}
	}
	return false
}

func requireChannelClosed(t *testing.T, ch <-chan struct{}, message string) {
	t.Helper()
	select {
	case <-ch:
	case <-time.After(time.Second):
		t.Fatal(message)
	}
}

type blockingStartProvider struct {
	started     chan context.Context
	release     chan struct{}
	starts      atomic.Int64
	statusCalls atomic.Int64
	status      idlewatchertypes.ContainerStatus
	startErr    error
}

var testWatcherID atomic.Uint64

func (*blockingStartProvider) ContainerPause(context.Context) error   { return nil }
func (*blockingStartProvider) ContainerUnpause(context.Context) error { return nil }
func (p *blockingStartProvider) ContainerStart(ctx context.Context) error {
	p.starts.Add(1)
	select {
	case p.started <- ctx:
	case <-ctx.Done():
		return context.Cause(ctx)
	}
	select {
	case <-p.release:
		return p.startErr
	case <-ctx.Done():
		return context.Cause(ctx)
	}
}
func (*blockingStartProvider) ContainerStop(context.Context, idlewatchertypes.Signal, int) error {
	return nil
}
func (*blockingStartProvider) ContainerKill(context.Context, idlewatchertypes.Signal) error {
	return nil
}
func (p *blockingStartProvider) ContainerStatus(context.Context) (idlewatchertypes.ContainerStatus, error) {
	p.statusCalls.Add(1)
	return p.status, nil
}
func (*blockingStartProvider) Watch(context.Context) (<-chan watcherEvents.Event, <-chan error) {
	return nil, nil
}
func (*blockingStartProvider) Close() {}

type observedDoneContext struct {
	context.Context
	doneObserved chan struct{}
	once         sync.Once
}

func (c *observedDoneContext) Done() <-chan struct{} {
	c.once.Do(func() { close(c.doneObserved) })
	return c.Context.Done()
}

type unhealthyHealthChecker struct {
	targetURL *url.URL
	cfg       health.HealthCheckConfig
}

func (*unhealthyHealthChecker) CheckHealth() (health.HealthCheckResult, error) {
	return health.HealthCheckResult{Healthy: false}, nil
}
func (c *unhealthyHealthChecker) URL() *url.URL                     { return c.targetURL }
func (c *unhealthyHealthChecker) Config() *health.HealthCheckConfig { return &c.cfg }
func (c *unhealthyHealthChecker) UpdateURL(targetURL *url.URL)      { c.targetURL = targetURL }
