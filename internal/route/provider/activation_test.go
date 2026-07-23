package provider

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/yusing/godoxy/internal/route"
	"github.com/yusing/godoxy/internal/routing"
	W "github.com/yusing/godoxy/internal/watcher"
	"github.com/yusing/goutils/task"
)

type activationTestWatcher struct {
	stream W.Stream
}

func (watcher activationTestWatcher) Watch(task.Parent) W.Stream {
	return watcher.stream
}

func newActivationTestStream(ready <-chan error) W.Stream {
	events := make(chan W.Event)
	errs := make(chan error)
	return W.Stream{Events: events, Errors: errs, Ready: ready}
}

func TestProviderActivationWaitsForWatcherReadiness(t *testing.T) {
	t.Run("ready empty provider", func(t *testing.T) {
		provider := NewStaticProvider("empty", route.Routes{})
		require.NoError(t, provider.LoadRoutes(t.Context()))
		runtimeTask := task.GetTestTask(t).Subtask("ready-runtime", false)

		activation := provider.Activate(runtimeTask)

		require.True(t, activation.EventLoopReady)
		require.NoError(t, activation.InfrastructureError)
		require.Equal(t, routing.ProviderActivationReady, activation.Health())
	})

	t.Run("initialization failure", func(t *testing.T) {
		sentinel := errors.New("watcher initialization failed")
		ready := make(chan error, 1)
		ready <- sentinel
		close(ready)
		provider := NewStaticProvider("failed-watcher", route.Routes{})
		provider.watcher = activationTestWatcher{stream: newActivationTestStream(ready)}
		require.NoError(t, provider.LoadRoutes(t.Context()))
		runtimeTask := task.GetTestTask(t).Subtask("failed-watcher-runtime", false)

		activation := provider.Activate(runtimeTask)

		require.False(t, activation.EventLoopReady)
		require.ErrorIs(t, activation.InfrastructureError, sentinel)
		require.Equal(t, routing.ProviderActivationFailed, activation.Health())
		require.NoError(t, context.Cause(runtimeTask.Context()))
	})

	t.Run("cancellation while initializing", func(t *testing.T) {
		ready := make(chan error)
		provider := NewStaticProvider("cancel-watcher", route.Routes{})
		provider.watcher = activationTestWatcher{stream: newActivationTestStream(ready)}
		require.NoError(t, provider.LoadRoutes(t.Context()))
		runtimeTask := task.GetTestTask(t).Subtask("cancel-watcher-runtime", false)

		done := make(chan routing.ProviderActivation, 1)
		go func() { done <- provider.Activate(runtimeTask) }()
		runtimeTask.Finish(nil)

		select {
		case activation := <-done:
			require.False(t, activation.EventLoopReady)
			require.ErrorIs(t, activation.InfrastructureError, context.Canceled)
			require.ErrorIs(t, context.Cause(runtimeTask.Context()), context.Canceled)
		case <-time.After(time.Second):
			t.Fatal("provider activation did not stop after cancellation")
		}
	})

	t.Run("malformed future stream fails closed", func(t *testing.T) {
		provider := NewStaticProvider("malformed-watcher", route.Routes{})
		provider.watcher = activationTestWatcher{}
		require.NoError(t, provider.LoadRoutes(t.Context()))
		runtimeTask := task.GetTestTask(t).Subtask("malformed-watcher-runtime", false)

		activation := provider.Activate(runtimeTask)

		require.ErrorIs(t, activation.InfrastructureError, ErrWatcherStreamUnavailable)
		require.False(t, activation.EventLoopReady)
	})
}

func TestProviderActivationCountsRoutesRejectedDuringValidation(t *testing.T) {
	provider := NewStaticProvider("invalid", route.Routes{
		"invalid": {
			Scheme: route.SchemeHTTP,
			Host:   "invalid.example.com",
			Port:   route.Port{Proxy: 80},
			Agent:  "missing-agent",
		},
	})

	err := provider.LoadRoutes(t.Context())
	require.ErrorContains(t, err, "agent pool not initialized")
	require.Zero(t, provider.NumRoutes(), "a route rejected during validation must not remain active")

	parent := task.RootTask("canceled-provider-activation", false)
	parent.FinishAndWait(context.Canceled)
	activation := provider.Activate(parent)

	require.Equal(t, 1, activation.DesiredRoutes)
	require.Zero(t, activation.AttemptedRoutes)
	require.Zero(t, activation.ActiveRoutes)
	require.Len(t, activation.FailedRoutes, 1)
	require.Equal(t, "invalid", activation.FailedRoutes[0].Route)
	require.ErrorContains(t, activation.FailedRoutes[0].Err, "agent pool not initialized")
	require.ErrorIs(t, activation.InfrastructureError, context.Canceled)
	require.Equal(t, routing.ProviderActivationFailed, activation.Health())
}
