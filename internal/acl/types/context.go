package acl

import "context"

type ContextKey struct{}

func SetCtx(ctx interface{ SetValue(any, any) }, acl ACL) {
	ctx.SetValue(ContextKey{}, acl)
}

func FromCtx(ctx context.Context) ACL {
	if acl, ok := ctx.Value(ContextKey{}).(ACL); ok {
		return acl
	}
	return nil
}
