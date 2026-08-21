package gphttp

import "context"

type nonUserRequestContextKey struct{}

// WithNonUserRequest marks a trusted in-process request that must not be
// treated as incoming user traffic.
func WithNonUserRequest(ctx context.Context) context.Context {
	return context.WithValue(ctx, nonUserRequestContextKey{}, struct{}{})
}

// IsNonUserRequest reports whether ctx belongs to trusted in-process traffic.
func IsNonUserRequest(ctx context.Context) bool {
	_, ok := ctx.Value(nonUserRequestContextKey{}).(struct{})
	return ok
}
