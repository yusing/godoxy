package config

import (
	"context"
	"errors"
	"iter"
	"net/http"

	"github.com/yusing/godoxy/internal/routing"
	"github.com/yusing/goutils/server"
	"github.com/yusing/goutils/task"
)

type State interface {
	InitFromFile(filename string) error
	Init(data []byte) error

	Task() *task.Task
	Context() context.Context

	Value() *Config

	Entrypoint() routing.Entrypoint
	ShortLinkMatcher() ShortLinkMatcher
	AutoCertProvider() server.CertProvider

	LoadOrStoreProvider(key string, value routing.Provider) (actual routing.Provider, loaded bool)
	DeleteProvider(key string)
	IterProviders() iter.Seq2[string, routing.Provider]
	NumProviders() int
	ActivateProviders(task.Parent) routing.ProviderActivationReport

	FlushTmpLog() error

	ActivateAPIServers(task.Parent) APIActivationReport
	RuntimeSnapshot() RuntimeSnapshot
	Stop(reason any)
}

// RuntimeMutationCoordinator serializes mutations of the authoritative
// runtime with configuration transitions. The returned release function must
// be called after the complete mutation transaction, including persistence and
// response construction, has finished.
type RuntimeMutationCoordinator interface {
	BeginRuntimeMutation(expected State) (release func(), err error)
}

// RuntimeStateSource provides the currently committed runtime to
// process-lifetime services that must follow configuration replacement.
type RuntimeStateSource interface {
	RuntimeState() State
}

type ShortLinkMatcher interface {
	AddRoute(alias string)
	DelRoute(alias string)
	ServeHTTP(w http.ResponseWriter, r *http.Request)
}

var ErrConfigChanged = errors.New("config changed")

var ErrRuntimeTransitioning = errors.New("runtime transition in progress")

type stateContextKey struct{}

func SetCtx(target interface{ SetValue(any, any) }, state State) {
	target.SetValue(stateContextKey{}, state)
}

func FromCtx(ctx context.Context) State {
	if ctx == nil {
		return nil
	}
	state, _ := ctx.Value(stateContextKey{}).(State)
	return state
}

type runtimeMutationCoordinatorContextKey struct{}

func SetRuntimeMutationCoordinator(target interface{ SetValue(any, any) }, coordinator RuntimeMutationCoordinator) {
	target.SetValue(runtimeMutationCoordinatorContextKey{}, coordinator)
}

func RuntimeMutationCoordinatorFromCtx(ctx context.Context) RuntimeMutationCoordinator {
	if ctx == nil {
		return nil
	}
	coordinator, _ := ctx.Value(runtimeMutationCoordinatorContextKey{}).(RuntimeMutationCoordinator)
	return coordinator
}

type runtimeStateSourceContextKey struct{}

func SetRuntimeStateSource(target interface{ SetValue(any, any) }, source RuntimeStateSource) {
	target.SetValue(runtimeStateSourceContextKey{}, source)
}

func RuntimeStateFromCtx(ctx context.Context) State {
	if ctx == nil {
		return nil
	}
	source, _ := ctx.Value(runtimeStateSourceContextKey{}).(RuntimeStateSource)
	if source == nil {
		return nil
	}
	return source.RuntimeState()
}
