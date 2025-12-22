//go:build !production

package idlewatcher

import (
	"math/rand/v2"
	"net/http"
	"time"

	"github.com/puzpuzpuz/xsync/v4"
	idlewatcher "github.com/yusing/godoxy/internal/idlewatcher/types"
	"github.com/yusing/godoxy/internal/types"
)

func DebugHandler(rw http.ResponseWriter, r *http.Request) {
	w := &Watcher{
		eventChs: xsync.NewMap[chan *WakeEvent, struct{}](),
		cfg: &types.IdlewatcherConfig{
			IdlewatcherProviderConfig: types.IdlewatcherProviderConfig{
				Docker: &types.DockerConfig{
					ContainerName: "test",
				},
			},
		},
	}

	switch r.URL.Path {
	case idlewatcher.LoadingPageCSSPath:
		serveStaticContent(rw, http.StatusOK, "text/css", cssBytes)
	case idlewatcher.LoadingPageJSPath:
		serveStaticContent(rw, http.StatusOK, "application/javascript", jsBytes)
	case idlewatcher.WakeEventsPath:
		go w.handleWakeEventsSSE(rw, r)
		ticker := time.NewTicker(1 * time.Second)
		defer ticker.Stop()
		events := []WakeEventType{
			WakeEventStarting,
			WakeEventWakingDep,
			WakeEventDepReady,
			WakeEventContainerWoke,
			WakeEventWaitingReady,
			WakeEventError,
			WakeEventReady,
		}
		messages := []string{
			"Starting",
			"Waking dependency",
			"Dependency ready",
			"Container woke",
			"Waiting for ready",
			"Error",
			"Ready",
		}

		for {
			select {
			case <-r.Context().Done():
				return
			case <-ticker.C:
				idx := rand.IntN(len(events))
				for ch := range w.eventChs.Range {
					ch <- &WakeEvent{
						Type:      string(events[idx]),
						Message:   messages[idx],
						Timestamp: time.Now(),
					}
				}
			}
		}
	default:
		w.writeLoadingPage(rw)
	}
}
