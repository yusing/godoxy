//go:build !production

package synk

import (
	"os"
	"os/signal"
	"runtime"
	"sync/atomic"
	"time"

	"github.com/rs/zerolog/log"
	"github.com/yusing/go-proxy/internal/utils/strutils"
)

var (
	numNonPooled, sizeNonPooled uint64
	numReused, sizeReused       uint64
	numGCed, sizeGCed           uint64
)

func addNonPooled(size int) {
	atomic.AddUint64(&numNonPooled, 1)
	atomic.AddUint64(&sizeNonPooled, uint64(size))
}

func addReused(size int) {
	atomic.AddUint64(&numReused, 1)
	atomic.AddUint64(&sizeReused, uint64(size))
}

func addGCed(size int) {
	atomic.AddUint64(&numGCed, 1)
	atomic.AddUint64(&sizeGCed, uint64(size))
}

var addCleanup = runtime.AddCleanup[[]byte, int]

func initPoolStats() {
	go func() {
		statsTicker := time.NewTicker(5 * time.Second)
		defer statsTicker.Stop()

		sig := make(chan os.Signal, 1)
		signal.Notify(sig, os.Interrupt)

		for {
			select {
			case <-sig:
				return
			case <-statsTicker.C:
				log.Info().
					Uint64("numReused", atomic.LoadUint64(&numReused)).
					Str("sizeReused", strutils.FormatByteSize(atomic.LoadUint64(&sizeReused))).
					Uint64("numGCed", atomic.LoadUint64(&numGCed)).
					Str("sizeGCed", strutils.FormatByteSize(atomic.LoadUint64(&sizeGCed))).
					Uint64("numNonPooled", atomic.LoadUint64(&numNonPooled)).
					Str("sizeNonPooled", strutils.FormatByteSize(atomic.LoadUint64(&sizeNonPooled))).
					Msg("bytes pool stats")
			}
		}
	}()
}
