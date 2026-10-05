package provider

import (
	"encoding/json/v2"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/yusing/godoxy/internal/docker"
	"github.com/yusing/godoxy/internal/routing"
	"github.com/yusing/godoxy/internal/types"
	"github.com/yusing/godoxy/internal/watcher"
	watcherEvents "github.com/yusing/godoxy/internal/watcher/events"
)

// Uses the real Docker list, label conversion, validation, and event handler.
func TestDockerProviderRemovesDestroyedContainerWithInvalidSibling(t *testing.T) {
	gin.SetMode(gin.TestMode)
	var candidate atomic.Bool
	summary := func(alias string) map[string]any {
		return map[string]any{
			"Id": alias + "-id", "Names": []string{"/" + alias},
			"Image": "nginx:latest", "ImageID": "sha256:test", "State": "running", "Status": "Up 1 hour",
			"Labels": map[string]string{
				docker.LabelAliases: alias,
				"proxy.*.scheme":    "http", "proxy.*.host": "192.0.2.41", "proxy.*.port": "8080",
				"proxy.*.healthcheck.disabled": "true", "proxy.*.homepage.show": "true",
			},
			"HostConfig":      map[string]any{"NetworkMode": "bridge"},
			"NetworkSettings": map[string]any{"Networks": map[string]any{"bridge": map[string]any{"IPAddress": "192.0.2.41"}}},
		}
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/_ping"):
			w.Header().Set("API-Version", "1.47")
			_, _ = w.Write([]byte("OK"))
		case strings.HasSuffix(r.URL.Path, "/containers/json"):
			w.Header().Set("Content-Type", "application/json")
			items := []map[string]any{summary("healthy")}
			if candidate.Load() {
				bad := summary("invalid")
				bad["Labels"].(map[string]string)[docker.LabelExclude] = "not-valid"
				items = append(items, bad)
			} else {
				items = append(items, summary("retired"))
			}
			_ = json.MarshalWrite(w, &items)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)
	runtimeTask := newRecoveryRuntime(t)
	p := NewDockerProvider("review", types.DockerProviderConfig{URL: strings.Replace(server.URL, "http://", "tcp://", 1)})
	p.watcher = noopWatcher{}
	require.NoError(t, p.LoadRoutes(runtimeTask.Context()))
	activation := p.Activate(runtimeTask)
	require.NoError(t, activation.InfrastructureError)
	require.Equal(t, 2, activation.ActiveRoutes)
	require.Equal(t, routing.ProviderTypeDocker, p.GetType())

	candidate.Store(true)
	p.handleEvents(runtimeTask, []watcher.Event{{Type: watcherEvents.EventTypeDocker, Action: watcherEvents.ActionContainerDestroy, ActorID: "retired-id", ActorName: "retired"}})
	require.ErrorContains(t, p.preparation.InfrastructureError, "not-valid")
	require.False(t, p.retryLoad, "partial errors do not schedule another load")
	_, exists := p.GetRoute("retired")
	require.False(t, exists, "a destroyed container must be removed after a successful complete Docker list despite an unrelated invalid sibling")
}
