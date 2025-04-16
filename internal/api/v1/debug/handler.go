//go:build debug

package debugapi

import (
	"iter"
	"net/http"
	"sort"
	"time"

	"github.com/yusing/go-proxy/agent/pkg/agent"
	config "github.com/yusing/go-proxy/internal/config/types"
	"github.com/yusing/go-proxy/internal/docker"
	"github.com/yusing/go-proxy/internal/idlewatcher"
	"github.com/yusing/go-proxy/internal/net/gphttp/gpwebsocket"
	"github.com/yusing/go-proxy/internal/net/gphttp/servemux"
	"github.com/yusing/go-proxy/internal/net/gphttp/server"
	"github.com/yusing/go-proxy/internal/proxmox"
	"github.com/yusing/go-proxy/internal/task"
)

func StartServer(cfg config.ConfigInstance) {
	srv := server.NewServer(server.Options{
		Name:     "debug",
		HTTPAddr: "127.0.0.1:7777",
		Handler:  newHandler(cfg),
	})
	srv.Start(task.RootTask("debug_server", false))
}

type debuggable interface {
	MarshalMap() map[string]any
	Key() string
}

func toSortedSlice[T debuggable](data iter.Seq2[string, T]) []map[string]any {
	s := make([]map[string]any, 0)
	for _, v := range data {
		m := v.MarshalMap()
		m["key"] = v.Key()
		s = append(s, m)
	}
	sort.Slice(s, func(i, j int) bool {
		return s[i]["key"].(string) < s[j]["key"].(string)
	})
	return s
}

func jsonHandler[T debuggable](getData iter.Seq2[string, T]) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		gpwebsocket.DynamicJSONHandler(w, r, func() []map[string]any {
			return toSortedSlice(getData)
		}, 1*time.Second)
	}
}

func iterMap[K comparable, V debuggable](m func() map[K]V) iter.Seq2[K, V] {
	return func(yield func(K, V) bool) {
		for k, v := range m() {
			if !yield(k, v) {
				break
			}
		}
	}
}

func newHandler(cfg config.ConfigInstance) http.Handler {
	mux := servemux.NewServeMux(cfg)
	mux.HandleFunc("GET", "/tasks", jsonHandler(task.AllTasks()))
	mux.HandleFunc("GET", "/idlewatcher", jsonHandler(idlewatcher.Watchers()))
	mux.HandleFunc("GET", "/agents", jsonHandler(agent.Agents.Iter))
	mux.HandleFunc("GET", "/proxmox", jsonHandler(proxmox.Clients.Iter))
	mux.HandleFunc("GET", "/docker", jsonHandler(iterMap(docker.Clients)))
	return mux
}
