package rules

import (
	"reflect"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/yusing/godoxy/internal/serialization"
)

func TestRulesValidate(t *testing.T) {
	tests := []struct {
		name  string
		rules string
		want  error
	}{
		{
			name: "no default rule",
			rules: `
- name: rule1
  on: header Host example.com
  do: pass
      `,
		},
		{
			name: "multiple default rules",
			rules: `
- name: default
  do: pass
- name: rule1
  on: default
  do: pass
      `,
			want: ErrMultipleDefaultRules,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var rules Rules
			convertible, err := serialization.ConvertString(strings.TrimSpace(tt.rules), reflect.ValueOf(&rules))
			require.True(t, convertible)

			if tt.want == nil {
				assert.NoError(t, err)
				return
			}
			assert.ErrorIs(t, err, tt.want)
		})
	}
}
