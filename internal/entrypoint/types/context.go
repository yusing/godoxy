package entrypoint

import (
	"context"
)

type ContextKey struct{}

func SetCtx(ctx interface{ SetValue(any, any) }, ep Entrypoint) {
	ctx.SetValue(ContextKey{}, ep)
}

func FromCtx(ctx context.Context) Entrypoint {
	if ep, ok := ctx.Value(ContextKey{}).(Entrypoint); ok {
		return ep
	}
	return nil
}
