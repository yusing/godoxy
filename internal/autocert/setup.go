package autocert

import (
	"errors"
	"os"

	"github.com/rs/zerolog/log"
	"github.com/yusing/godoxy/internal/utils/strutils"
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

	for _, expiry := range p.GetExpiries() {
		log.Info().Msg("certificate expire on " + strutils.FormatTime(expiry))
		break
	}

	return nil
}
