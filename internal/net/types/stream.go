package nettypes

import (
	"context"
	"net"
)

type Stream interface {
	ListenAndServe(ctx context.Context, preDial PreDialFunc)
	LocalAddr() net.Addr
	Close() error
}

type PreDialFunc func(ctx context.Context) error
