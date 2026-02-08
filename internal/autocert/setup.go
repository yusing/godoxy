package autocert

import (
	gperr "github.com/yusing/goutils/errs"
)

func (p *Provider) setupExtraProviders() error {
	p.sniMatcher = sniMatcher{}
	if len(p.cfg.Extra) == 0 {
		return nil
	}

	p.extraProviders = make([]*Provider, 0, len(p.cfg.Extra))

	errs := gperr.NewBuilder("setup extra providers error")
	for _, extra := range p.cfg.Extra {
		user, legoCfg, err := extra.AsConfig().GetLegoConfig()
		if err != nil {
			errs.Add(p.fmtError(err))
			continue
		}
		ep, err := NewProvider(extra.AsConfig(), user, legoCfg)
		if err != nil {
			errs.Add(p.fmtError(err))
			continue
		}
		p.extraProviders = append(p.extraProviders, ep)
	}
	return errs.Error()
}
