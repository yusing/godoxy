package serialization

import (
	"testing"

	"github.com/goccy/go-yaml"
	"github.com/stretchr/testify/require"
)

func TestUnmarshalValidateEmptyDocument(t *testing.T) {
	type Nested struct {
		Enabled bool
	}
	type Target struct {
		Timeout int
		Name    string
		Nested  Nested
	}

	defaults := Target{Timeout: 3, Name: "default", Nested: Nested{Enabled: true}}

	t.Run("empty document keeps the values already in the target", func(t *testing.T) {
		for name, data := range map[string][]byte{
			"nil":           nil,
			"blank":         []byte(""),
			"whitespace":    []byte("\n  \n"),
			"comments only": []byte("# nothing to see here\n"),
		} {
			t.Run(name, func(t *testing.T) {
				target := defaults
				require.NoError(t, UnmarshalValidate(data, &target, yaml.Unmarshal))
				require.Equal(t, defaults, target)
			})
		}
	})

	t.Run("document overrides only the fields it names", func(t *testing.T) {
		target := defaults
		require.NoError(t, UnmarshalValidate([]byte("name: overridden\n"), &target, yaml.Unmarshal))
		require.Equal(t, Target{Timeout: 3, Name: "overridden", Nested: Nested{Enabled: true}}, target)
	})

	t.Run("an explicit zero still wins over the value in the target", func(t *testing.T) {
		target := defaults
		require.NoError(t, UnmarshalValidate([]byte("timeout: 0\nnested:\n  enabled: false\n"), &target, yaml.Unmarshal))
		require.Equal(t, Target{Timeout: 0, Name: "default"}, target)
	})
}
