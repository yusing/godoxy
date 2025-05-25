package main

import (
	"os"
	"sync"

	"github.com/rs/zerolog/log"
	"github.com/yusing/go-proxy/internal/auth"
	"github.com/yusing/go-proxy/internal/common"
	"github.com/yusing/go-proxy/internal/config"
	"github.com/yusing/go-proxy/internal/dnsproviders"
	"github.com/yusing/go-proxy/internal/gperr"
	"github.com/yusing/go-proxy/internal/homepage"
	"github.com/yusing/go-proxy/internal/logging"
	"github.com/yusing/go-proxy/internal/logging/memlogger"
	"github.com/yusing/go-proxy/internal/metrics/systeminfo"
	"github.com/yusing/go-proxy/internal/metrics/uptime"
	"github.com/yusing/go-proxy/internal/net/gphttp/middleware"
	"github.com/yusing/go-proxy/internal/task"
	"github.com/yusing/go-proxy/pkg"
)

func parallel(fns ...func()) {
	var wg sync.WaitGroup
	for _, fn := range fns {
		wg.Add(1)
		go func() {
			defer wg.Done()
			fn()
		}()
	}
	wg.Wait()
}

func main() {
	initProfiling()

	logging.InitLogger(os.Stderr, memlogger.GetMemLogger())
	log.Info().Msgf("GoDoxy version %s", pkg.GetVersion())
	log.Trace().Msg("trace enabled")
	parallel(
		dnsproviders.InitProviders,
		homepage.InitIconListCache,
		systeminfo.Poller.Start,
		middleware.LoadComposeFiles,
	)

	if common.APIJWTSecret == nil {
		log.Warn().Msg("API_JWT_SECRET is not set, using random key")
		common.APIJWTSecret = common.RandomJWTKey()
	}

	for _, dir := range common.RequiredDirectories {
		prepareDirectory(dir)
	}

	cfg, err := config.Load()
	if err != nil {
		gperr.LogWarn("errors in config", err)
	}

	cfg.Start(&config.StartServersOptions{
		Proxy: true,
	})
	if err := auth.Initialize(); err != nil {
		log.Fatal().Err(err).Msg("failed to initialize authentication")
	}
	// API Handler needs to start after auth is initialized.
	cfg.StartServers(&config.StartServersOptions{
		API: true,
	})

	uptime.Poller.Start()
	config.WatchChanges()

	task.WaitExit(cfg.Value().TimeoutShutdown)
}

func prepareDirectory(dir string) {
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		if err = os.MkdirAll(dir, 0o755); err != nil {
			log.Fatal().Msgf("failed to create directory %s: %v", dir, err)
		}
	}
}
