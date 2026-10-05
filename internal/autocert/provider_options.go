package autocert

import (
	"encoding/json/jsontext"
	"encoding/json/v2"
	"fmt"

	"github.com/yusing/godoxy/internal/serialization"
	strutils "github.com/yusing/goutils/strings"
)

// ProviderOptions keeps credentials redacted while accepting the concrete scalar,
// list, and nested configuration fields exposed by lego providers.
type ProviderOptions map[string]strutils.Redacted

func (options *ProviderOptions) UnmarshalMap(src serialization.SerializedObject) error {
	result := make(ProviderOptions, len(src))
	for key, value := range src {
		switch value := value.(type) {
		case nil:
			result[key] = ""
		case string:
			result[key] = strutils.Redacted(value)
		case strutils.Redacted:
			result[key] = value
		default:
			encoded, err := json.Marshal(&value)
			if err != nil {
				return fmt.Errorf("encode provider option %s: %w", key, err)
			}
			result[key] = strutils.Redacted(encoded)
		}
	}
	*options = result
	return nil
}

func (options *ProviderOptions) UnmarshalJSON(data []byte) error {
	var src map[string]jsontext.Value
	if err := json.Unmarshal(data, &src); err != nil {
		return err
	}
	result := make(ProviderOptions, len(src))
	for key, value := range src {
		if value.Kind() == '"' {
			var text string
			if err := json.Unmarshal(value, &text); err != nil {
				return err
			}
			result[key] = strutils.Redacted(text)
		} else if value.Kind() == 'n' {
			result[key] = ""
		} else {
			result[key] = strutils.Redacted(value)
		}
	}
	*options = result
	return nil
}
