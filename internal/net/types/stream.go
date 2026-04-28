package nettypes

import (
	"context"
	"net"
)

type Stream interface {
	ListenAndServe(ctx context.Context, preDial, onRead HookFunc) error
	LocalAddr() net.Addr
	Close() error
}

type ConnProxy interface {
	ProxyConn(ctx context.Context, conn net.Conn)
}

type HookFunc func(ctx context.Context) error
