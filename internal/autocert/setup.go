package autocert

import (
	"errors"
	"fmt"
	"os"

	"github.com/rs/zerolog/log"
	gperr "github.com/yusing/goutils/errs"
	strutils "github.com/yusing/goutils/strings"
)

func (p *Provider) Setup() (err error) {
	if err = p.LoadCert(); err != nil {
		if !errors.Is(err, os.ErrNotExist) { // ignore if cert doesn't exist
			return err
		}
		log.Debug().Msg("obtaining cert due to error loading cert")
		if err = p.ObtainCert(); err != nil {
			return err
		}
	}

	if err = p.setupExtraProviders(); err != nil {
		return err
	}

	for _, expiry := range p.GetExpiries() {
		log.Info().Msg("certificate expire on " + strutils.FormatTime(expiry))
		break
	}

	return nil
}

func (p *Provider) setupExtraProviders() error {
	p.extraProviders = nil
	p.sniMatcher = sniMatcher{}
	if len(p.cfg.Extra) == 0 {
		p.rebuildSNIMatcher()
		return nil
	}

	for i := range p.cfg.Extra {
		merged := mergeExtraConfig(p.cfg, &p.cfg.Extra[i])
		user, legoCfg, err := merged.GetLegoConfig()
		if err != nil {
			return err.Subjectf("extra[%d]", i)
		}
		ep := NewProvider(&merged, user, legoCfg)
		if err := ep.Setup(); err != nil {
			return gperr.PrependSubject(fmt.Sprintf("extra[%d]", i), err)
		}
		p.extraProviders = append(p.extraProviders, ep)
	}
	p.rebuildSNIMatcher()
	return nil
}

func mergeExtraConfig(mainCfg *Config, extraCfg *Config) Config {
	merged := *mainCfg
	merged.Extra = nil
	merged.CertPath = extraCfg.CertPath
	merged.KeyPath = extraCfg.KeyPath

	if merged.Email == "" {
		merged.Email = mainCfg.Email
	}

	if len(extraCfg.Domains) > 0 {
		merged.Domains = extraCfg.Domains
	}
	if extraCfg.ACMEKeyPath != "" {
		merged.ACMEKeyPath = extraCfg.ACMEKeyPath
	}
	if extraCfg.Provider != "" {
		merged.Provider = extraCfg.Provider
	}
	if len(extraCfg.Options) > 0 {
		merged.Options = extraCfg.Options
	}
	if len(extraCfg.Resolvers) > 0 {
		merged.Resolvers = extraCfg.Resolvers
	}
	if extraCfg.CADirURL != "" {
		merged.CADirURL = extraCfg.CADirURL
	}
	if len(extraCfg.CACerts) > 0 {
		merged.CACerts = extraCfg.CACerts
	}
	if extraCfg.EABKid != "" {
		merged.EABKid = extraCfg.EABKid
	}
	if extraCfg.EABHmac != "" {
		merged.EABHmac = extraCfg.EABHmac
	}
	if extraCfg.HTTPClient != nil {
		merged.HTTPClient = extraCfg.HTTPClient
	}
	return merged
}
