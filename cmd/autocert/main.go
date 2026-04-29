package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"

	"github.com/goccy/go-yaml"
	"github.com/rs/zerolog/log"
	"github.com/yusing/godoxy/internal/autocert"
	"github.com/yusing/godoxy/internal/common"
	"github.com/yusing/godoxy/internal/dnsproviders"
	"github.com/yusing/godoxy/internal/serialization"
)

type configFile struct {
	AutoCert *autocert.Config `json:"autocert"`
}

func main() {
	dnsproviders.InitProviders()
	autocert.UseLocalAutocertOperations()

	if err := run(os.Args[1:]); err != nil {
		log.Fatal().Err(err).Msg("autocert command failed")
	}
}

func run(args []string) error {
	if len(args) == 0 {
		return errors.New("missing command")
	}

	switch args[0] {
	case "obtain":
		return runObtain(args[1:])
	default:
		return fmt.Errorf("unknown command %q", args[0])
	}
}

func runObtain(args []string) error {
	fs := flag.NewFlagSet("obtain", flag.ContinueOnError)
	fs.SetOutput(io.Discard)

	configPath := fs.String("config", common.ConfigPath, "")
	certPath := fs.String("cert-path", "", "")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *certPath == "" {
		return errors.New("missing --cert-path")
	}

	cfg, err := loadConfig(*configPath)
	if err != nil {
		return err
	}
	selected, err := findAutocertConfig(cfg.AutoCert, *certPath)
	if err != nil {
		return err
	}

	return obtainCert(selected)
}

func loadConfig(path string) (*configFile, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	cfg := new(configFile)
	if err := serialization.UnmarshalValidate(data, cfg, yaml.Unmarshal); err != nil {
		return nil, err
	}
	return cfg, nil
}

func findAutocertConfig(cfg *autocert.Config, certPath string) (*autocert.Config, error) {
	if cfg == nil {
		return nil, errors.New("autocert config not found")
	}
	if cfg.CertPath == certPath {
		return cfg, nil
	}
	for i := range cfg.Extra {
		extra := cfg.Extra[i].AsConfig()
		if extra.CertPath == certPath {
			return extra, nil
		}
	}
	return nil, fmt.Errorf("autocert provider not found for cert path %q", certPath)
}
