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
  scheme: http
  host: localhost
  port: 2375
  tls:
    ca_file: /etc/ssl/ca.crt
    cert_file: /etc/ssl/cert.crt
    key_file: /etc/ssl/key.crt`), &cfg)
		assert.Error(t, err, os.ErrNotExist)
		assert.Equal(t, &DockerProviderConfig{URL: "http://localhost:2375", TLS: &DockerTLSConfig{CAFile: "/etc/ssl/ca.crt", CertFile: "/etc/ssl/cert.crt", KeyFile: "/etc/ssl/key.crt"}}, cfg["test"])
	})
}

func TestDockerProviderConfigValidation(t *testing.T) {
	tests := []struct {
		name    string
		yamlStr string
		wantErr bool
	}{
		{name: "valid url (http)", yamlStr: "test: http://localhost:2375", wantErr: false},
		{name: "valid url (https)", yamlStr: "test: https://localhost:2375", wantErr: false},
		{name: "valid url (tcp)", yamlStr: "test: tcp://localhost:2375", wantErr: false},
		{name: "valid url (tls)", yamlStr: "test: tls://localhost:2375", wantErr: false},
		{name: "valid url (unix)", yamlStr: "test: unix:///var/run/docker.sock", wantErr: false},
		{name: "valid url (ssh)", yamlStr: "test: ssh://localhost:2375", wantErr: false},
		{name: "invalid url", yamlStr: "test: ftp://localhost/2375", wantErr: true},
		{name: "valid scheme", yamlStr: `
        test:
          scheme: http
          host: localhost
          port: 2375
        `, wantErr: false},
		{name: "invalid scheme", yamlStr: `
        test:
          scheme: invalid
          host: localhost
          port: 2375
        `, wantErr: true},
		{name: "valid host (ipv4)", yamlStr: `
        test:
          scheme: http
          host: 127.0.0.1
          port: 2375
        `, wantErr: false},
		{name: "valid host (ipv6)", yamlStr: `
        test:
          scheme: http
          host: ::1
          port: 2375
        `, wantErr: false},
		{name: "valid host (hostname)", yamlStr: `
        test:
          scheme: http
          host: example.com
          port: 2375
        `, wantErr: false},
		{name: "invalid host", yamlStr: `
        test:
          scheme: http
          host: invalid:1234
          port: 2375
        `, wantErr: true},
		{name: "valid port", yamlStr: `
        test:
          scheme: http
          host: localhost
          port: 2375
        `, wantErr: false},
		{name: "invalid port", yamlStr: `
        test:
          scheme: http
          host: localhost
          port: 65536
        `, wantErr: true},
		{name: "valid tls", yamlStr: `
        test:
          scheme: tls
          host: localhost
          port: 2375
          tls:
            ca_file: /etc/ssl/ca.crt
            cert_file: /etc/ssl/cert.crt
            key_file: /etc/ssl/key.crt
        `, wantErr: false},
		{name: "valid tls (only ca file)", yamlStr: `
        test:
          scheme: tls
          host: localhost
          port: 2375
          tls:
            ca_file: /etc/ssl/ca.crt
        `, wantErr: false},
		{name: "invalid tls (missing cert file)", yamlStr: `
        test:
          scheme: tls
          host: localhost
          port: 2375
          tls:
            key_file: /etc/ssl/key.crt
        `, wantErr: true},
		{name: "invalid tls (missing key file)", yamlStr: `
        test:
          scheme: tls
          host: localhost
          port: 2375
          tls:
            cert_file: /etc/ssl/cert.crt
        `, wantErr: true},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var cfg map[string]*DockerProviderConfig
			err := serialization.UnmarshalValidateYAML([]byte(test.yamlStr), &cfg)
			if test.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}
