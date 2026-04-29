package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/goccy/go-yaml"
	"github.com/rs/zerolog/log"
	"github.com/yusing/godoxy/internal/autocert"
	"github.com/yusing/godoxy/internal/dnsproviders"
	"github.com/yusing/godoxy/internal/logging"
	"github.com/yusing/godoxy/internal/logging/memlogger"
	"github.com/yusing/godoxy/internal/serialization"
)

func main() {
	var mode string
	var configPath string
	flag.StringVar(&mode, "mode", string(autocert.RunModeObtainIfMissing), "helper mode: obtain-if-missing or renew-all")
	flag.StringVar(&configPath, "config", autocert.RuntimeConfigPath(), "autocert config snapshot path")
	flag.Parse()

	logging.InitLogger(os.Stderr, memlogger.GetMemLogger())
	dnsproviders.InitProviders()

	data, err := os.ReadFile(configPath)
	if err != nil {
		fail(err)
	}

	var cfg autocert.Config
	if err := serialization.UnmarshalValidate(data, &cfg, yaml.Unmarshal); err != nil {
		fail(err)
	}

	user, legoCfg, err := cfg.GetLegoConfig()
	if err != nil {
		fail(err)
	}

	provider, err := autocert.NewProvider(&cfg, user, legoCfg)
	if err != nil {
		fail(err)
	}

	var runErr error
	switch autocert.RunMode(mode) {
	case autocert.RunModeObtainIfMissing:
		runErr = provider.ObtainCertIfNotExistsAll()
	case autocert.RunModeRenewAll:
		runErr = provider.ObtainCertAll()
	default:
		runErr = fmt.Errorf("unknown mode %q", mode)
	}
	if runErr != nil {
		fail(runErr)
	}
	provider.PrintCertExpiriesAll()
}

func fail(err error) {
	log.Error().Err(err).Msg("autocert helper failed")
	os.Exit(1)
}
