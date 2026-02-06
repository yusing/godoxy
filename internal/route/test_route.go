package route

import (
	"github.com/yusing/godoxy/internal/types"
	"github.com/yusing/goutils/task"
)

func NewTestRoute[T interface{ Helper() }](t T, task task.Parent, base *Route) (types.Route, error) {
	t.Helper()

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
