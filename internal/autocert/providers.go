package autocert

import (
	"github.com/go-acme/lego/v4/challenge"
	"github.com/yusing/go-proxy/internal/gperr"
	"github.com/yusing/go-proxy/internal/serialization"
)

type Generator func(map[string]any) (challenge.Provider, gperr.Error)

var Providers = make(map[string]Generator)

func DNSProvider[CT any, PT challenge.Provider](
	defaultCfg func() *CT,
	newProvider func(*CT) (PT, error),
) Generator {
	return func(opt map[string]any) (challenge.Provider, gperr.Error) {
		cfg := defaultCfg()
		err := serialization.MapUnmarshalValidate(opt, &cfg)
		if err != nil {
			return nil, err
		}
		p, pErr := newProvider(cfg)
		return p, gperr.Wrap(pErr)
	}
}
