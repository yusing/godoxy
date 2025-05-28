package server

import (
	"context"
	"errors"
	"net"
	"net/http"

	"github.com/rs/zerolog"
)

func convertError(err error) error {
	switch {
	case err == nil, errors.Is(err, http.ErrServerClosed), errors.Is(err, context.Canceled), errors.Is(err, net.ErrClosed):
		return nil
	default:
		return err
	}
}

func HandleError(logger *zerolog.Logger, err error, msg string) {
	logger.Fatal().Err(err).Msg(msg)
}
