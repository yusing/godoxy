package route

import (
	"testing"

	"github.com/yusing/godoxy/internal/entrypoint"
	epctx "github.com/yusing/godoxy/internal/entrypoint/types"
	"github.com/yusing/godoxy/internal/types"
	"github.com/yusing/goutils/task"
)

func NewStartedTestRoute(t testing.TB, base *Route) (types.Route, error) {
	t.Helper()

	task := task.GetTestTask(t)
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

	return base.impl, nil
}
