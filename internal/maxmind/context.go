package maxmind

import "context"

type contextKey struct{}

func SetCtx(target interface{ SetValue(any, any) }, instance *MaxMind) {
	target.SetValue(contextKey{}, instance)
}

func FromCtx(ctx context.Context) *MaxMind {
	instance, _ := ctx.Value(contextKey{}).(*MaxMind)
	return instance
}
