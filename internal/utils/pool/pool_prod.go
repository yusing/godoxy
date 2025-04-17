//go:build !debug

package pool

func (p Pool[T]) checkExists(key string) {
	// no-op in production
}
