package synk

import (
	"slices"
	"testing"
)

var sizes = []int{1024, 4096, 16384, 65536, 32 * 1024, 128 * 1024, 512 * 1024, 1024 * 1024, 2 * 1024 * 1024}

func BenchmarkBytesPool_GetSmall(b *testing.B) {
	for b.Loop() {
		bytesPool.Put(bytesPool.GetSized(1024))
	}
}

func BenchmarkBytesPool_MakeSmall(b *testing.B) {
	for b.Loop() {
		_ = make([]byte, 1024)
	}
}

func BenchmarkBytesPool_GetLarge(b *testing.B) {
	for b.Loop() {
		buf := bytesPool.GetSized(DropThreshold / 2)
		buf[0] = 1
		bytesPool.Put(buf)
	}
}

func BenchmarkBytesPool_GetLargeUnsized(b *testing.B) {
	for b.Loop() {
		buf := slices.Grow(bytesPool.Get(), DropThreshold/2)
		buf = append(buf, 1)
		bytesPool.Put(buf)
	}
}

func BenchmarkBytesPool_MakeLarge(b *testing.B) {
	for b.Loop() {
		buf := make([]byte, DropThreshold/2)
		buf[0] = 1
		_ = buf
	}
}

func BenchmarkBytesPool_GetAll(b *testing.B) {
	for i := range b.N {
		bytesPool.Put(bytesPool.GetSized(sizes[i%len(sizes)]))
	}
}

func BenchmarkBytesPool_GetAllUnsized(b *testing.B) {
	for i := range b.N {
		bytesPool.Put(slices.Grow(bytesPool.Get(), sizes[i%len(sizes)]))
	}
}

func BenchmarkBytesPool_MakeAll(b *testing.B) {
	for i := range b.N {
		_ = make([]byte, sizes[i%len(sizes)])
	}
}

func BenchmarkBytesPoolWithMemory(b *testing.B) {
	pool := GetBytesPoolWithUniqueMemory()
	for i := range b.N {
		pool.Put(slices.Grow(pool.Get(), sizes[i%len(sizes)]))
	}
}
