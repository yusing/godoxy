package entrypoint

import (
	"context"

	"github.com/yusing/godoxy/internal/routing"
)

func SetCtx(ctx interface{ SetValue(key any, value any) }, ep routing.Entrypoint) {
	routing.SetEntrypointCtx(ctx, ep)
}

func FromCtx(ctx context.Context) routing.Entrypoint {
	return routing.EntrypointFromCtx(ctx)
}
