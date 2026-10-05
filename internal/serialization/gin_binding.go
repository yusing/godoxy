package serialization

import (
	"io"
	"net/http"

	"github.com/goccy/go-yaml"
	strutils "github.com/yusing/goutils/strings"
)

type (
	GinJSONBinding struct{}
	GinYAMLBinding struct{}
)

// Name implements binding.Binding.
func (b GinJSONBinding) Name() string {
	return "json"
}

// Bind implements binding.Binding.
func (b GinJSONBinding) Bind(req *http.Request, obj any) error {
	m := make(map[string]any)
	if err := strutils.NewJSONDecoder(req.Body).Decode(&m); err != nil {
		return err
	}
	m, err := substituteEnv(m)
	if err != nil {
		return err
	}
	return MapUnmarshalValidate(m, obj)
}

// Name implements binding.Binding.
func (b GinYAMLBinding) Name() string {
	return "yaml"
}

// Bind implements binding.Binding.
func (b GinYAMLBinding) Bind(req *http.Request, obj any) error {
	m := make(map[string]any)
	data, err := io.ReadAll(req.Body)
	if err != nil {
		return err
	}
	if err := unmarshalEnv(data, &m, yaml.Unmarshal); err != nil {
		return err
	}
	return MapUnmarshalValidate(m, obj)
}
