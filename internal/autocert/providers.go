package autocert

import (
	"github.com/go-acme/lego/v5/challenge"
	"github.com/yusing/godoxy/internal/serialization"
	strutils "github.com/yusing/goutils/strings"
	"reflect"
)

type Generator struct {
	ConfigType reflect.Type
	New        func(map[string]strutils.Redacted) (challenge.Provider, error)
}

var Providers = make(map[string]Generator)

func DNSProvider[CT any, PT challenge.Provider](
	defaultCfg func() *CT,
	newProvider func(*CT) (PT, error),
) Generator {
	return Generator{ConfigType: reflect.TypeFor[CT](), New: func(opt map[string]strutils.Redacted) (challenge.Provider, error) {
		cfg := defaultCfg()
		if len(opt) > 0 {
			err := serialization.MapUnmarshalValidate(serialization.ToSerializedObject(opt), &cfg)
			if err != nil {
				return nil, err
			}
		}
		p, pErr := newProvider(cfg)
		return p, pErr
	}}
}
