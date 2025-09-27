package synk

import (
	"sync/atomic"
	"unsafe"
	"weak"
)

type weakBuf = weak.Pointer[[]byte]

func makeWeak(b *[]byte) weakBuf {
	return weak.Make(b)
}

func getBufFromWeak(w weakBuf) []byte {
	ptr := w.Value()
	if ptr != nil {
		return *ptr
	}
	return nil
}

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

type sliceInternal struct {
	ptr unsafe.Pointer
	len int
	cap int
}

func sliceStruct(b *[]byte) *sliceInternal {
	return (*sliceInternal)(unsafe.Pointer(b))
}

func underlyingPtr(b []byte) unsafe.Pointer {
	return sliceStruct(&b).ptr
}

func setCap(b *[]byte, cap int) {
	sliceStruct(b).cap = cap
}

func setLen(b *[]byte, len int) {
	sliceStruct(b).len = len
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
			addNonPooled(p.initSize)
			return make([]byte, 0, p.initSize)
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
			addReused(cap(bPtr))
			return bPtr
		default:
			addNonPooled(size)
			return make([]byte, 0, size)
		}
	}
}

func (p *BytesPool) GetSized(size int) []byte {
	for {
		select {
		case bWeak := <-p.sizedPool:
			b := getBufFromWeak(bWeak)
			if b == nil {
				continue
			}
			capB := cap(b)

			remainingSize := capB - size
			if remainingSize == 0 {
				addReused(capB)
				return b[:size]
			}

			if remainingSize > 0 { // capB > size (buffer larger than requested)
				addReused(size)

				p.Put(b[size:capB])

				// return the first part and limit the capacity to the requested size
				ret := b[:size]
				setLen(&ret, size)
				setCap(&ret, size)
				return ret
			}

			// size is not enough
			select {
			case p.sizedPool <- bWeak:
			default:
				addDropped(cap(b))
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
		addDropped(size)
		return
	}
	b = b[:0]
	if size >= SizedPoolThreshold {
		p.put(size, makeWeak(&b), p.sizedPool)
	} else {
		p.put(size, makeWeak(&b), p.unsizedPool)
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
		addDropped(int(capB))
		return
	}
	b = b[:0]
	w := makeWeak(&b)
	select {
	case p.pool <- w:
	default:
		addDropped(int(capB))
		// just drop it
	}
}

//go:inline
func (p *BytesPool) put(size int, w weakBuf, pool chan weakBuf) {
	select {
	case pool <- w:
	default:
		addDropped(size)
		// just drop it
	}
}

func init() {
	initPoolStats()
}
