package main

import (
	"context"
	"fmt"
	"os"
	"runtime/debug"
	"sync"
	"time"

	"github.com/rs/zerolog/log"
	"github.com/yusing/godoxy/internal/api"
	"github.com/yusing/godoxy/internal/auth"
	"github.com/yusing/godoxy/internal/common"
	"github.com/yusing/godoxy/internal/config"
	configtypes "github.com/yusing/godoxy/internal/config/types"
	"github.com/yusing/godoxy/internal/dnsproviders"
	"github.com/yusing/godoxy/internal/health"
	"github.com/yusing/godoxy/internal/health/monitor"
	iconlist "github.com/yusing/godoxy/internal/homepage/icons/list"
	"github.com/yusing/godoxy/internal/logging"
	"github.com/yusing/godoxy/internal/logging/memlogger"
	"github.com/yusing/godoxy/internal/net/gphttp/middleware"
	"github.com/yusing/godoxy/internal/route"
	"github.com/yusing/godoxy/internal/route/rules"
	"github.com/yusing/godoxy/internal/routevalidate"
	"github.com/yusing/godoxy/internal/routing"
	"github.com/yusing/goutils/task"
	"github.com/yusing/goutils/version"
)

func parallel(fns ...func()) {
	var wg sync.WaitGroup
	for _, fn := range fns {
		wg.Go(fn)
	}
	wg.Wait()
}

func main() {
	defer func() {
		if r := recover(); r != nil {
			fmt.Printf("panic: %v\n\n%s", r, debug.Stack())

			// Force the OS streams to sync/flush before exiting
			os.Stderr.Sync()
			os.Stdout.Sync()

			os.Exit(2)
		}
	}()

	done := make(chan struct{}, 1)
	go func() {
		select {
		case <-done:
			return
		case <-time.After(common.InitTimeout):
			log.Fatal().Msgf("timeout waiting for initialization to complete, exiting...")
		}
	}()

	initProfiling()

	logging.InitLogger(os.Stderr, memlogger.GetMemLogger())
	log.Info().Msgf("GoDoxy version %s", version.Get())
	log.Trace().Msg("trace enabled")
	parallel(
		dnsproviders.InitProviders,
		iconlist.InitCache,
		middleware.LoadComposeFiles,
	)

	if common.APIJWTSecret == nil {
		log.Warn().Msg("API_JWT_SECRET is not set, using random key")
		common.APIJWTSecret = common.RandomJWTKey()
	}

	for _, dir := range common.RequiredDirectories {
		prepareDirectory(dir)
	}

	route.InitBuilder(routevalidate.Validate)
	route.InitHealthMonitor(func(r routing.Route) health.HealthMonitor {
		return monitor.NewMonitor(r)
	})

	rules.InitAuthHandler(auth.AuthOrProceed)
	result := config.Load(initializeRuntimeServices)
	if !initialRuntimeReady(result) {
		// RuntimeManager already emitted the complete lifecycle report. Keep
		// process policy separate so issue diagnostics are rendered exactly once.
		log.Fatal().Msg("runtime activation failed, exiting")
	}

	listenDebugServer()

	config.WatchChanges()

	close(done)

	task.WaitExit(config.ShutdownTimeout())
}

func initializeRuntimeServices(ctx context.Context) error {
	if err := auth.Initialize(ctx); err != nil {
		return fmt.Errorf("initialize authentication: %w", err)
	}
	if err := context.Cause(ctx); err != nil {
		return err
	}
	if err := api.RegisterHandlers(); err != nil {
		return fmt.Errorf("register API handlers: %w", err)
	}
	return nil
}

func initialRuntimeReady(result configtypes.ReloadResult) bool {
	if !result.Committed {
		return false
	}
	return result.Health == configtypes.ActivationHealthy || result.Health == configtypes.ActivationDegraded
}

func prepareDirectory(dir string) {
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		if err = os.MkdirAll(dir, 0o755); err != nil {
			log.Fatal().Msgf("failed to create directory %s: %v", dir, err)
		}
	}
}
