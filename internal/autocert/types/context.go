package autocert

import "context"

type ContextKey struct{}

func SetCtx(ctx interface{ SetValue(any, any) }, p Provider) {
	ctx.SetValue(ContextKey{}, p)
}

func FromCtx(ctx context.Context) Provider {
	if provider, ok := ctx.Value(ContextKey{}).(Provider); ok {
		return provider
	}
	return nil
}
