package routetest

import (
	"testing"

	"github.com/yusing/godoxy/internal/entrypoint"
	epctx "github.com/yusing/godoxy/internal/entrypoint"
	"github.com/yusing/godoxy/internal/route"
	"github.com/yusing/godoxy/internal/routing"
	"github.com/yusing/goutils/task"
)

func NewStartedRoute(tb testing.TB, base *route.Route) (routing.Route, error) {
	tb.Helper()

	task := task.GetTestTask(tb)
	if ep := epctx.FromCtx(task.Context()); ep == nil {
		ep = entrypoint.NewEntrypoint(task, nil)
		epctx.SetCtx(task, ep)
	}

	err := base.Validate()
	if err != nil {
		return nil, err
	}

	err = base.Start(task)
	if err != nil {
		return nil, err
	}

	return base.Impl(), nil
}
