package autocert

import (
	"github.com/go-acme/lego/v4/challenge"
	"github.com/yusing/godoxy/internal/serialization"
	gperr "github.com/yusing/goutils/errs"
	strutils "github.com/yusing/goutils/strings"
)

type Generator func(map[string]strutils.Redacted) (challenge.Provider, gperr.Error)

var Providers = make(map[string]Generator)

func DNSProvider[CT any, PT challenge.Provider](
	defaultCfg func() *CT,
	newProvider func(*CT) (PT, error),
) Generator {
	return func(opt map[string]strutils.Redacted) (challenge.Provider, gperr.Error) {
		cfg := defaultCfg()
		if len(opt) > 0 {
			err := serialization.MapUnmarshalValidate(serialization.ToSerializedObject(opt), &cfg)
			if err != nil {
				return nil, err
			}
		}
		p, pErr := newProvider(cfg)
		return p, gperr.Wrap(pErr)
	}
}
