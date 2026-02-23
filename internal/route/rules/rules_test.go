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
header Host example.com {
	pass
}`,
		},
		{
			name: "multiple default rules",
			rules: `
default {
	pass
}

default {
	pass
}`,
			want: ErrMultipleDefaultRules,
		},
		{
			name: "multiple responses on same condition",
			rules: `
header Host example.com {
	error 404 "not found"
}

header Host example.com {
	error 403 "forbidden"
}
`,
			want: ErrDeadRule,
		},
		{
			name: "same condition different formatting error then proxy",
			rules: `
header Host example.com & method GET {
	error 404 "not found"
}

method GET
header Host example.com {
	proxy http://127.0.0.1:8080
}
`,
			want: ErrDeadRule,
		},
		{
			name: "same condition with non terminating first rule",
			rules: `
header Host example.com {
	set resp_header X-Test first
}

header Host example.com {
	error 403 "forbidden"
}
`,
			want: nil,
		},
		{
			name: "same condition with terminating handler inside if block",
			rules: `
header Host example.com {
	default {
		error 404 "not found"
	}
}

header Host example.com {
	error 403 "forbidden"
}
`,
			want: ErrDeadRule,
		},
		{
			name: "same condition with terminating handler across if else block",
			rules: `
header Host example.com {
	method GET {
		error 404 "not found"
	} else {
		redirect https://example.com
	}
}

header Host example.com {
	error 403 "forbidden"
}
`,
			want: ErrDeadRule,
		},
		{
			name: "same condition with non terminating if branch in if else block",
			rules: `
header Host example.com {
	method GET {
		set resp_header X-Test first
	} else {
		error 404 "not found"
	}
}

header Host example.com {
	error 403 "forbidden"
}
`,
			want: nil,
		},
		{
			name: "unconditional terminating rule shadows later unconditional rule",
			rules: `
{
	error 404 "not found"
}

{
	error 403 "forbidden"
}
`,
			want: ErrDeadRule,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var rules Rules
			convertible, err := serialization.ConvertString(strings.TrimSpace(tt.rules), reflect.ValueOf(&rules))
			require.True(t, convertible)
			require.NoError(t, err)

			err = rules.Validate()

			if tt.want == nil {
				assert.NoError(t, err)
				return
			}
			assert.ErrorIs(t, err, tt.want)
		})
	}
}

func TestHasTopLevelLBrace(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want bool
	}{
		{
			name: "escaped quote inside double quoted string",
			in:   `"test\"more{"`,
			want: false,
		},
		{
			name: "escaped quote inside single quoted string",
			in:   "'test\\'more{'",
			want: false,
		},
		{
			name: "top-level brace outside quoted string",
			in:   `"test\"more" {`,
			want: true,
		},
		{
			name: "backtick keeps existing behavior",
			in:   "`test\\`more{`",
			want: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, hasTopLevelLBrace(tt.in))
		})
	}
}

func TestRulesParse_BlockTriedThenYAMLFails_ReturnsBlockError(t *testing.T) {
	input := `default {`

	_, blockErr := parseBlockRules(input)
	require.Error(t, blockErr)

	var rules Rules
	err := rules.Parse(input)
	require.Error(t, err)
	assert.Equal(t, blockErr.Error(), err.Error())
}
