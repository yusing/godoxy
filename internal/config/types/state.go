package config

import (
	"context"
	"errors"
	"iter"
	"net/http"

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

	EntrypointHandler() http.Handler
	AutoCertProvider() server.CertProvider

	LoadOrStoreProvider(key string, value types.RouteProvider) (actual types.RouteProvider, loaded bool)
	DeleteProvider(key string)
	IterProviders() iter.Seq2[string, types.RouteProvider]
	NumProviders() int
	StartProviders() error

	FlushTmpLog()
}

// could be nil
var ActiveState synk.Value[State]

var ErrConfigChanged = errors.New("config changed")
