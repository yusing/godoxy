//go:build debug

package pool

import (
	"runtime/debug"

	"github.com/yusing/go-proxy/internal/logging"
)

func (p Pool[T]) checkExists(key string) {
	if _, ok := p.m.Load(key); ok {
		logging.Warn().Msgf("%s: key %s already exists\nstacktrace: %s", p.name, key, string(debug.Stack()))
	}
}
