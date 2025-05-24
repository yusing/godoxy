package synk

import (
	"runtime"
	"unsafe"
)

type weakBuf = unsafe.Pointer

func makeWeak(b *[]byte) weakBuf {
	ptr := runtime_registerWeakPointer(unsafe.Pointer(b))
	runtime.KeepAlive(ptr)
	addCleanup(b, addGCed, cap(*b))
	return weakBuf(ptr)
}

func getBufFromWeak(w weakBuf) []byte {
	ptr := (*[]byte)(runtime_makeStrongFromWeak(w))
	if ptr == nil {
		return nil
	}
	return *ptr
}

//go:linkname runtime_registerWeakPointer weak.runtime_registerWeakPointer
func runtime_registerWeakPointer(unsafe.Pointer) unsafe.Pointer

//go:linkname runtime_makeStrongFromWeak weak.runtime_makeStrongFromWeak
func runtime_makeStrongFromWeak(unsafe.Pointer) unsafe.Pointer

type BytesPool struct {
	pool     chan weakBuf
	initSize int
}

const (
	kb = 1024
	mb = 1024 * kb
)

const (
	InPoolLimit = 32 * mb

	DefaultInitBytes  = 4 * kb
	PoolThreshold     = 256 * kb
	DropThresholdHigh = 4 * mb

	PoolSize = InPoolLimit / PoolThreshold
)

var bytesPool = &BytesPool{
	pool:     make(chan weakBuf, PoolSize),
	initSize: DefaultInitBytes,
}

func NewBytesPool() *BytesPool {
	return bytesPool
}

func (p *BytesPool) Get() []byte {
	for {
		select {
		case bWeak := <-p.pool:
			bPtr := getBufFromWeak(bWeak)
			if bPtr == nil {
				continue
			}
			addReused(cap(bPtr))
			return bPtr
		default:
			return make([]byte, 0, p.initSize)
		}
	}
}

func (p *BytesPool) GetSized(size int) []byte {
	if size <= PoolThreshold {
		return make([]byte, size)
	}
	for {
		select {
		case bWeak := <-p.pool:
			bPtr := getBufFromWeak(bWeak)
			if bPtr == nil {
				continue
			}
			capB := cap(bPtr)
			if capB >= size {
				addReused(capB)
				return (bPtr)[:size]
			}
			select {
			case p.pool <- bWeak:
			default:
				// just drop it
			}
		default:
		}
		return make([]byte, size)
	}
}

func (p *BytesPool) Put(b []byte) {
	size := cap(b)
	if size <= PoolThreshold || size > DropThresholdHigh {
		return
	}
	b = b[:0]
	w := makeWeak(&b)
	select {
	case p.pool <- w:
	default:
		// just drop it
	}
}

func init() {
	initPoolStats()
}
