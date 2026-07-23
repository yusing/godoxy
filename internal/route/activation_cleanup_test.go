package route

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yusing/goutils/task"
)

type failedStartRoute struct {
	*Route
	err          error
	startCalls   atomic.Int32
	cleanupCalls atomic.Int32
}

func (route *failedStartRoute) Start(parent task.Parent) error {
	route.startCalls.Add(1)
	route.Route.Init(parent, "failed-route", true)
	route.Task().OnCancel("record cleanup", func() {
		route.cleanupCalls.Add(1)
	})
	return route.err
}

func TestRouteStartCleansFailedComponentExactlyOnce(t *testing.T) {
	startErr := errors.New("listener collision")
	base := &Route{Alias: "colliding"}
	base.ForceConflictWin = true
	base.started = make(chan struct{})
	implementation := &failedStartRoute{Route: base, err: startErr}
	base.impl = implementation
	parent := task.GetTestTask(t).Subtask("provider", true)

	require.ErrorIs(t, base.Start(parent), startErr)
	require.ErrorIs(t, base.Start(parent), startErr)
	require.EqualValues(t, 1, implementation.startCalls.Load())
	require.EqualValues(t, 1, implementation.cleanupCalls.Load())
	require.ErrorIs(t, context.Cause(implementation.Task().Context()), startErr)
}
