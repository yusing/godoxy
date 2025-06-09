//go:build debug

package stream

import (
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

func logDebugf(stream zerolog.LogObjectMarshaler, format string, v ...any) {
	log.Debug().Object("stream", stream).Msgf(format, v...)
}
