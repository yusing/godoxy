package common

import (
	"crypto/rand"
	"encoding/base64"

	"github.com/rs/zerolog/log"
)

func decodeJWTKey(key string) []byte {
	if key == "" {
		return nil
	}
	bytes, err := base64.StdEncoding.DecodeString(key)
	if err != nil {
		log.Panic().Err(err).Msg("failed to decode jwt key")
	}
	return bytes
}

func RandomJWTKey() []byte {
	key := make([]byte, 32)
	_, err := rand.Read(key)
	if err != nil {
		log.Panic().Err(err).Msg("failed to generate random jwt key")
	}
	return key
}
