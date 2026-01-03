package types

import (
	"encoding/json"
	"fmt"
	"net"
	"net/url"
	"os"
	"strconv"

	"github.com/yusing/godoxy/internal/common"
	"github.com/yusing/godoxy/internal/serialization"
	"github.com/yusing/goutils/env"
	gperr "github.com/yusing/goutils/errs"
)

type DockerProviderConfig struct {
	URL string           `json:"url,omitempty"`
	TLS *DockerTLSConfig `json:"tls,omitempty"`
} // @name DockerProviderConfig

type DockerProviderConfigDetailed struct {
	Scheme string           `json:"scheme,omitempty" validate:"required,oneof=http https tcp tls unix ssh"`
	Host   string           `json:"host,omitempty" validate:"required,hostname|ip"`
	Port   int              `json:"port,omitempty" validate:"required,min=1,max=65535"`
	TLS    *DockerTLSConfig `json:"tls" validate:"omitempty"`
}

type DockerTLSConfig struct {
	CAFile   string `json:"ca_file,omitempty" validate:"required"`
	CertFile string `json:"cert_file,omitempty" validate:"required_with=KeyFile"`
	KeyFile  string `json:"key_file,omitempty" validate:"required_with=CertFile"`
} // @name DockerTLSConfig

func (cfg *DockerProviderConfig) MarshalJSON() ([]byte, error) {
	return json.Marshal(cfg.URL)
}

func (cfg *DockerProviderConfig) Parse(value string) error {
	if value == common.DockerHostFromEnv {
		cfg.URL = env.GetEnvString("DOCKER_HOST", "unix:///var/run/docker.sock")
		return nil
	}

	u, err := url.Parse(value)
	if err != nil {
		return err
	}

	switch u.Scheme {
	case "http", "https", "tcp", "tls":
		cfg.URL = u.String()
	case "unix", "ssh":
		cfg.URL = value
	default:
		return fmt.Errorf("invalid scheme: %s", u.Scheme)
	}

	return nil
}

func (cfg *DockerProviderConfig) UnmarshalMap(m map[string]any) gperr.Error {
	var tmp DockerProviderConfigDetailed
	var err = serialization.MapUnmarshalValidate(m, &tmp)
	if err != nil {
		return err
	}

	cfg.URL = fmt.Sprintf("%s://%s", tmp.Scheme, net.JoinHostPort(tmp.Host, strconv.Itoa(tmp.Port)))
	cfg.TLS = tmp.TLS
	if cfg.TLS != nil {
		if err := checkFilesOk(cfg.TLS.CAFile, cfg.TLS.CertFile, cfg.TLS.KeyFile); err != nil {
			return gperr.Wrap(err)
		}
	}
	return nil
}

func checkFilesOk(files ...string) error {
	if common.IsTest {
		return nil
	}
	var errs gperr.Builder
	for _, file := range files {
		if _, err := os.Stat(file); err != nil {
			errs.Add(err)
		}
	}
	return errs.Error()
}
