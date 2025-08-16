package agentapi

import (
	"crypto/rand"
	"encoding/base64"
	"sync/atomic"
	"time"

	"github.com/rs/zerolog/log"
	"github.com/yusing/go-proxy/agent/pkg/agent"
)

type PEMPairResponse struct {
	Cert string `json:"cert" format:"base64"`
	Key  string `json:"key" format:"base64"`
} // @name PEMPairResponse

var encryptionKey atomic.Value

const rotateKeyInterval = 15 * time.Minute

func init() {
	if err := rotateKey(); err != nil {
		log.Panic().Err(err).Msg("failed to generate encryption key")
	}
	go func() {
		for range time.Tick(rotateKeyInterval) {
			if err := rotateKey(); err != nil {
				log.Error().Err(err).Msg("failed to rotate encryption key")
			}
		}
	}()
}

func getEncryptionKey() []byte {
	return encryptionKey.Load().([]byte)
}

func rotateKey() error {
	// generate a random 32 bytes key
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		return err
	}
	encryptionKey.Store(key)
	return nil
}

func toPEMPairResponse(encPEMPair agent.PEMPair) PEMPairResponse {
	return PEMPairResponse{
		Cert: base64.StdEncoding.EncodeToString(encPEMPair.Cert),
		Key:  base64.StdEncoding.EncodeToString(encPEMPair.Key),
	}
}

func fromEncryptedPEMPairResponse(pemPair PEMPairResponse) (agent.PEMPair, error) {
	encCert, err := base64.StdEncoding.DecodeString(pemPair.Cert)
	if err != nil {
		return agent.PEMPair{}, err
	}
	encKey, err := base64.StdEncoding.DecodeString(pemPair.Key)
	if err != nil {
		return agent.PEMPair{}, err
	}
	pair := agent.PEMPair{Cert: encCert, Key: encKey}
	return pair.Decrypt(getEncryptionKey())
}
