package config

import (
	"context"
	"errors"
	"iter"
	"net/http"

	entrypoint "github.com/yusing/godoxy/internal/entrypoint/types"
	"github.com/yusing/godoxy/internal/types"
	"github.com/yusing/goutils/server"
	"github.com/yusing/goutils/synk"
	"github.com/yusing/goutils/task"
)

type State interface {
	InitFromFile(filename string) error
	Init(data []byte) error

	Task() *task.Task
	Context() context.Context

	Value() *Config

	Entrypoint() entrypoint.Entrypoint
	ShortLinkMatcher() ShortLinkMatcher
	AutoCertProvider() server.CertProvider

	LoadOrStoreProvider(key string, value types.RouteProvider) (actual types.RouteProvider, loaded bool)
	DeleteProvider(key string)
	IterProviders() iter.Seq2[string, types.RouteProvider]
	NumProviders() int
	StartProviders() error

	FlushTmpLog()

	StartAPIServers()
	StartMetrics()
}

type ShortLinkMatcher interface {
	AddRoute(alias string)
	DelRoute(alias string)
	ServeHTTP(w http.ResponseWriter, r *http.Request)
}

// could be nil before first call on Load
var ActiveState synk.Value[State]

// working state while loading config, same as ActiveState after successful load
var WorkingState synk.Value[State]

var ErrConfigChanged = errors.New("config changed")
