package common

import (
	"crypto/rand"
	"encoding/base64"
	"log"
)

func decodeJWTKey(key string) []byte {
	if key == "" {
		return nil
	}
	bytes, err := base64.StdEncoding.DecodeString(key)
	if err != nil {
		log.Fatalf("failed to decode secret: %s", err)
	}
	return bytes
}

func RandomJWTKey() []byte {
	key := make([]byte, 32)
	_, err := rand.Read(key)
	if err != nil {
		log.Fatalf("failed to generate random jwt key: %s", err)
	}
	return key
}
