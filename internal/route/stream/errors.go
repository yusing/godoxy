package stream

import (
	"context"
	"errors"
	"io"
	"syscall"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

func convertErr(err error) error {
	if err == nil {
		return nil
	}
	switch {
	case errors.Is(err, context.Canceled),
		errors.Is(err, io.ErrClosedPipe),
		errors.Is(err, syscall.ECONNRESET):
		return nil
	default:
		return err
	}
}

func logErr(stream zerolog.LogObjectMarshaler, err error, msg string) {
	err = convertErr(err)
	if err == nil {
		return
	}
	log.Err(err).Object("stream", stream).Msg(msg)
}

func logErrf(stream zerolog.LogObjectMarshaler, err error, format string, v ...any) {
	err = convertErr(err)
	if err == nil {
		return
	}
	log.Err(err).Object("stream", stream).Msgf(format, v...)
}
