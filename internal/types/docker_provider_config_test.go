package types

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/yusing/godoxy/internal/serialization"
)

func TestDockerProviderConfigUnmarshalMap(t *testing.T) {
	t.Run("string", func(t *testing.T) {
		var cfg map[string]*DockerProviderConfig
		err := serialization.UnmarshalValidateYAML([]byte("test: http://localhost:2375"), &cfg)
		assert.NoError(t, err)
		assert.Equal(t, &DockerProviderConfig{URL: "http://localhost:2375"}, cfg["test"])
	})

	t.Run("detailed", func(t *testing.T) {
		var cfg map[string]*DockerProviderConfig
		err := serialization.UnmarshalValidateYAML([]byte(`
test:
  host: localhost
  port: 2375
  protocol: http
  tls:
    ca_file: /etc/ssl/ca.crt
    cert_file: /etc/ssl/cert.crt
    key_file: /etc/ssl/key.crt`), &cfg)
		assert.Error(t, err, os.ErrNotExist)
		assert.Equal(t, &DockerProviderConfig{URL: "http://localhost:2375", TLS: &TLSConfig{CAFile: "/etc/ssl/ca.crt", CertFile: "/etc/ssl/cert.crt", KeyFile: "/etc/ssl/key.crt"}}, cfg["test"])
	})
}
