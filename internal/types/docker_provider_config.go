package types

import (
	"encoding/json"
	"fmt"
	"net"
	"os"
	"strconv"

	"github.com/yusing/godoxy/internal/serialization"
	gperr "github.com/yusing/goutils/errs"
)

type DockerProviderConfig struct {
	URL string     `json:"url,omitempty"`
	TLS *TLSConfig `json:"tls,omitempty"`
} // @name DockerProviderConfig

type DockerProviderConfigDetailed struct {
	Host     string     `json:"host,omitempty"`
	Port     int        `json:"port,omitempty"`
	Protocol string     `json:"protocol,omitempty"`
	TLS      *TLSConfig `json:"tls"`
}

func (cfg *DockerProviderConfig) MarshalJSON() ([]byte, error) {
	return json.Marshal(cfg.URL)
}

func (cfg *DockerProviderConfig) UnmarshalJSON(data []byte) error {
	// either a string or a map
	var v any
	if err := json.Unmarshal(data, &v); err != nil {
		return err
	}
	switch v := v.(type) {
	case string:
		cfg.URL = v
	case map[string]any:
		var tmp DockerProviderConfigDetailed
		var err error = serialization.MapUnmarshalValidate(v, &tmp)
		if err != nil {
			return err
		}

		cfg.URL = fmt.Sprintf("%s://%s", tmp.Protocol, net.JoinHostPort(tmp.Host, strconv.Itoa(tmp.Port)))
		cfg.TLS = tmp.TLS
		if err := checkFilesOk(cfg.TLS.CAFile, cfg.TLS.CertFile, cfg.TLS.KeyFile); err != nil {
			return err
		}
	}
	return nil
}

func checkFilesOk(files ...string) error {
	var errs gperr.Builder
	for _, file := range files {
		if _, err := os.Stat(file); err != nil {
			errs.Add(err)
		}
	}
	return errs.Error()
}
