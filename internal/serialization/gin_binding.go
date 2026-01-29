package serialization

import (
	"net/http"

	"github.com/bytedance/sonic"
	"github.com/goccy/go-yaml"
)

type (
	GinJSONBinding struct{}
	GinYAMLBinding struct{}
)

func (b GinJSONBinding) Name() string {
	return "json"
}

func (b GinJSONBinding) Bind(req *http.Request, obj any) error {
	m := make(map[string]any)
	if err := sonic.ConfigDefault.NewDecoder(NewSubstituteEnvReader(req.Body)).Decode(&m); err != nil {
		return err
	}
	return MapUnmarshalValidate(m, obj)
}

func (b GinYAMLBinding) Name() string {
	return "yaml"
}

func (b GinYAMLBinding) Bind(req *http.Request, obj any) error {
	m := make(map[string]any)
	if err := yaml.NewDecoder(NewSubstituteEnvReader(req.Body)).Decode(&m); err != nil {
		return err
	}
	return MapUnmarshalValidate(m, obj)
}
