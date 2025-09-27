//go:build pprof

package main

import (
	"net/http"
	_ "net/http/pprof"
	"runtime"
	"runtime/debug"
	"time"

	"github.com/rs/zerolog/log"
	"github.com/yusing/godoxy/internal/utils/strutils"
)

func initProfiling() {
	go func() {
		log.Info().Msgf("pprof server started at http://localhost:7777/debug/pprof/")
		log.Error().Err(http.ListenAndServe(":7777", nil)).Msg("pprof server failed")
	}()
	go func() {
		ticker := time.NewTicker(time.Second * 10)
		defer ticker.Stop()

		var m runtime.MemStats
		var gcStats debug.GCStats

		for range ticker.C {
			runtime.ReadMemStats(&m)
			debug.ReadGCStats(&gcStats)

			log.Info().Msgf("-----------------------------------------------------")
			log.Info().Msgf("Timestamp: %s", time.Now().Format(time.RFC3339))
			log.Info().Msgf("  Go Heap - In Use (Alloc/HeapAlloc): %s", strutils.FormatByteSize(m.Alloc))
			log.Info().Msgf("  Go Heap - Reserved from OS (HeapSys): %s", strutils.FormatByteSize(m.HeapSys))
			log.Info().Msgf("  Go Stacks - In Use (StackInuse): %s", strutils.FormatByteSize(m.StackInuse))
			log.Info().Msgf("  Go Runtime - Other Sys (MSpanInuse, MCacheInuse, BuckHashSys, GCSys, OtherSys): %s", strutils.FormatByteSize(m.MSpanInuse+m.MCacheInuse+m.BuckHashSys+m.GCSys+m.OtherSys))
			log.Info().Msgf("  Go Runtime - Total from OS (Sys): %s", strutils.FormatByteSize(m.Sys))
			log.Info().Msgf("  Go Runtime - Freed from OS (HeapReleased): %s", strutils.FormatByteSize(m.HeapReleased))
			log.Info().Msgf("  Number of Goroutines: %d", runtime.NumGoroutine())
			log.Info().Msgf("  Number of completed GC cycles: %d", m.NumGC)
			log.Info().Msgf("  Number of GCs: %d", gcStats.NumGC)
			log.Info().Msgf("  Total GC time: %s", gcStats.PauseTotal)
			log.Info().Msgf("  Last GC time: %s", gcStats.LastGC.Format(time.DateTime))
			log.Info().Msg("-----------------------------------------------------")
		}
	}()
}
