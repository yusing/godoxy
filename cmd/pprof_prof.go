//go:build pprof

package main

import (
	"net/http"
	_ "net/http/pprof"
	"runtime"
	"runtime/debug"
	"time"

	"github.com/rs/zerolog/log"
	"github.com/yusing/go-proxy/internal/utils/strutils"
)

const mb = 1024 * 1024

func initProfiling() {
	debug.SetGCPercent(-1)
	debug.SetMemoryLimit(50 * mb)
	debug.SetMaxStack(4 * mb)

	go func() {
		log.Info().Msgf("pprof server started at http://localhost:7777/debug/pprof/")
		log.Error().Err(http.ListenAndServe(":7777", nil)).Msg("pprof server failed")
	}()
	go func() {
		ticker := time.NewTicker(time.Second * 10)
		defer ticker.Stop()
		for range ticker.C {
			var m runtime.MemStats
			runtime.ReadMemStats(&m)
			log.Info().Msgf("-----------------------------------------------------")
			log.Info().Msgf("Timestamp: %s", time.Now().Format(time.RFC3339))
			log.Info().Msgf("  Go Heap - In Use (Alloc/HeapAlloc): %s", strutils.FormatByteSize(m.Alloc))
			log.Info().Msgf("  Go Heap - Reserved from OS (HeapSys): %s", strutils.FormatByteSize(m.HeapSys))
			log.Info().Msgf("  Go Stacks - In Use (StackInuse): %s", strutils.FormatByteSize(m.StackInuse))
			log.Info().Msgf("  Go Runtime - Other Sys (MSpanInuse, MCacheInuse, BuckHashSys, GCSys, OtherSys): %s", strutils.FormatByteSize(m.MSpanInuse+m.MCacheInuse+m.BuckHashSys+m.GCSys+m.OtherSys))
			log.Info().Msgf("  Go Runtime - Total from OS (Sys): %s", strutils.FormatByteSize(m.Sys))
			log.Info().Msgf("  Number of Goroutines: %d", runtime.NumGoroutine())
			log.Info().Msgf("  Number of GCs: %d", m.NumGC)
			log.Info().Msg("-----------------------------------------------------")
		}
	}()
}
