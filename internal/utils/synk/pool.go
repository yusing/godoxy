package synk

import (
	"os"
	"os/signal"
	"sync/atomic"
	"time"

	"github.com/yusing/go-proxy/internal/logging"
)

type BytesPool struct {
	pool     chan []byte
	initSize int
}

const (
	kb = 1024
	mb = 1024 * kb
)

const (
	InPoolLimit = 32 * mb

	DefaultInitBytes  = 32 * kb
	PoolThreshold     = 64 * kb
	DropThresholdHigh = 4 * mb

	PoolSize = InPoolLimit / PoolThreshold

	CleanupInterval   = 5 * time.Second
	MaxDropsPerCycle  = 10
	MaxChecksPerCycle = 100
)

var bytesPool = &BytesPool{
	pool:     make(chan []byte, PoolSize),
	initSize: DefaultInitBytes,
}

func NewBytesPool() *BytesPool {
	return bytesPool
}

func (p *BytesPool) Get() []byte {
	select {
	case b := <-p.pool:
		subInPoolSize(int64(cap(b)))
		return b
	default:
		return make([]byte, 0, p.initSize)
	}
}

func (p *BytesPool) GetSized(size int) []byte {
	if size <= PoolThreshold {
		return make([]byte, size)
	}
	select {
	case b := <-p.pool:
		if size <= cap(b) {
			subInPoolSize(int64(cap(b)))
			return b[:size]
		}
		select {
		case p.pool <- b:
			addInPoolSize(int64(cap(b)))
		default:
		}
	default:
	}
	return make([]byte, size)
}

func (p *BytesPool) Put(b []byte) {
	size := cap(b)
	if size > DropThresholdHigh || poolFull() {
		return
	}
	b = b[:0]
	select {
	case p.pool <- b:
		addInPoolSize(int64(size))
		return
	default:
		// just drop it
	}
}

var inPoolSize int64

func addInPoolSize(size int64) {
	atomic.AddInt64(&inPoolSize, size)
}

func subInPoolSize(size int64) {
	atomic.AddInt64(&inPoolSize, -size)
}

func init() {
	// Periodically drop some buffers to prevent excessive memory usage
	go func() {
		sigCh := make(chan os.Signal, 1)
		signal.Notify(sigCh, os.Interrupt)

		cleanupTicker := time.NewTicker(CleanupInterval)
		defer cleanupTicker.Stop()

		for {
			select {
			case <-cleanupTicker.C:
				dropBuffers()
			case <-sigCh:
				return
			}
		}
	}()
}

func poolFull() bool {
	return atomic.LoadInt64(&inPoolSize) >= InPoolLimit
}

// dropBuffers removes excess buffers from the pool when it grows too large.
func dropBuffers() {
	// Check if pool has more than a threshold of buffers
	count := 0
	droppedSize := 0
	checks := 0
	for count < MaxDropsPerCycle && checks < MaxChecksPerCycle && atomic.LoadInt64(&inPoolSize) > InPoolLimit*2/3 {
		select {
		case b := <-bytesPool.pool:
			n := cap(b)
			subInPoolSize(int64(n))
			droppedSize += n
			count++
		default:
			time.Sleep(10 * time.Millisecond)
		}
		checks++
	}
	if count > 0 {
		logging.Debug().Int("dropped", count).Int("size", droppedSize).Msg("dropped buffers from pool")
	}
}
