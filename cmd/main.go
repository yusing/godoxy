package main

import (
	"errors"
	"os"
	"sync"
	"time"

	"github.com/rs/zerolog/log"
	"github.com/yusing/godoxy/internal/auth"
	"github.com/yusing/godoxy/internal/common"
	"github.com/yusing/godoxy/internal/config"
	"github.com/yusing/godoxy/internal/dnsproviders"
	iconlist "github.com/yusing/godoxy/internal/homepage/icons/list"
	"github.com/yusing/godoxy/internal/logging"
	"github.com/yusing/godoxy/internal/logging/memlogger"
	"github.com/yusing/godoxy/internal/net/gphttp/middleware"
	"github.com/yusing/godoxy/internal/route/rules"
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

	err := config.Load()
	if err != nil {
		if criticalErr, ok := errors.AsType[config.CriticalError](err); ok {
			log.Fatal().Err(criticalErr).Msg("critical error in config")
		}
		log.Warn().Err(err).Msg("errors in config")
	}

	if err := auth.Initialize(); err != nil {
		log.Fatal().Err(err).Msg("failed to initialize authentication")
	}
	rules.InitAuthHandler(auth.AuthOrProceed)

	listenDebugServer()

	config.WatchChanges()

	close(done)

	task.WaitExit(config.Value().TimeoutShutdown)
}

func prepareDirectory(dir string) {
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		if err = os.MkdirAll(dir, 0o755); err != nil {
			log.Fatal().Msgf("failed to create directory %s: %v", dir, err)
		}
	}
}
