package synk

import (
	"runtime"
	"sync/atomic"
	"unsafe"
)

type weakBuf = unsafe.Pointer

func makeWeak(b *[]byte) weakBuf {
	ptr := runtime_registerWeakPointer(unsafe.Pointer(b))
	addCleanup(b, addGCed, cap(*b))
	runtime.KeepAlive(ptr)
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
	sizedPool   chan weakBuf
	unsizedPool chan weakBuf
	initSize    int
}

type BytesPoolWithMemory struct {
	maxAllocatedSize atomic.Uint32
	numShouldShrink  atomic.Int32
	pool             chan weakBuf
}

const (
	kb = 1024
	mb = 1024 * kb
)

const (
	InPoolLimit = 32 * mb

	UnsizedAvg         = 4 * kb
	SizedPoolThreshold = 256 * kb
	DropThreshold      = 4 * mb

	SizedPoolSize   = InPoolLimit * 8 / 10 / SizedPoolThreshold
	UnsizedPoolSize = InPoolLimit * 2 / 10 / UnsizedAvg

	ShouldShrinkThreshold = 10
)

var bytesPool = &BytesPool{
	sizedPool:   make(chan weakBuf, SizedPoolSize),
	unsizedPool: make(chan weakBuf, UnsizedPoolSize),
	initSize:    UnsizedAvg,
}

var bytesPoolWithMemory = make(chan weakBuf, UnsizedPoolSize)

func GetBytesPool() *BytesPool {
	return bytesPool
}

func GetBytesPoolWithUniqueMemory() *BytesPoolWithMemory {
	return &BytesPoolWithMemory{
		pool: bytesPoolWithMemory,
	}
}

func (p *BytesPool) Get() []byte {
	for {
		select {
		case bWeak := <-p.unsizedPool:
			bPtr := getBufFromWeak(bWeak)
			if bPtr == nil {
				continue
			}
			addReused(cap(bPtr))
			return bPtr
		default:
			return make([]byte, 0)
		}
	}
}

func (p *BytesPoolWithMemory) Get() []byte {
	for {
		size := int(p.maxAllocatedSize.Load())
		select {
		case bWeak := <-p.pool:
			bPtr := getBufFromWeak(bWeak)
			if bPtr == nil {
				continue
			}
			capB := cap(bPtr)
			addReused(capB)
			return bPtr
		default:
			addNonPooled(size)
			return make([]byte, 0, size)
		}
	}
}

func (p *BytesPool) GetSized(size int) []byte {
	if size <= SizedPoolThreshold {
		addNonPooled(size)
		return make([]byte, size)
	}
	for {
		select {
		case bWeak := <-p.sizedPool:
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
			case p.sizedPool <- bWeak:
			default:
				// just drop it
			}
		default:
		}
		addNonPooled(size)
		return make([]byte, size)
	}
}

func (p *BytesPool) Put(b []byte) {
	size := cap(b)
	if size > DropThreshold {
		return
	}
	b = b[:0]
	w := makeWeak(&b)
	if size <= SizedPoolThreshold {
		p.put(w, p.unsizedPool)
	} else {
		p.put(w, p.sizedPool)
	}
}

func (p *BytesPoolWithMemory) Put(b []byte) {
	capB := uint32(cap(b))

	for {
		current := p.maxAllocatedSize.Load()

		if capB < current {
			// Potential shrink case
			if p.numShouldShrink.Add(1) > ShouldShrinkThreshold {
				if p.maxAllocatedSize.CompareAndSwap(current, capB) {
					p.numShouldShrink.Store(0) // reset counter
					break
				}
				p.numShouldShrink.Add(-1) // undo if CAS failed
			}
			break
		} else if capB > current {
			// Growing case
			if p.maxAllocatedSize.CompareAndSwap(current, capB) {
				break
			}
			// retry if CAS failed
		} else {
			// equal case - no change needed
			break
		}
	}

	if capB > DropThreshold {
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

//go:inline
func (p *BytesPool) put(w weakBuf, pool chan weakBuf) {
	select {
	case pool <- w:
	default:
		// just drop it
	}
}

func init() {
	initPoolStats()
}
