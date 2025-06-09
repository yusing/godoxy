package nettypes

import (
	"context"
	"net"
)

type Stream interface {
	ListenAndServe(ctx context.Context, preDial, onRead HookFunc)
	LocalAddr() net.Addr
	Close() error
}

type HookFunc func(ctx context.Context) error
