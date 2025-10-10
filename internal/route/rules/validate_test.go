package rules

import (
	"testing"

	expect "github.com/yusing/goutils/testing"
)

func TestExtractExpr(t *testing.T) {
	tests := []struct {
		name     string
		in       string
		wantT    MatcherType
		wantExpr string
	}{
		{
			name:     "string implicit",
			in:       "foo",
			wantT:    MatcherTypeString,
			wantExpr: "foo",
		},
		{
			name:     "string explicit",
			in:       "string(`foo`)",
			wantT:    MatcherTypeString,
			wantExpr: "foo",
		},
		{
			name:     "glob",
			in:       "glob(foo)",
			wantT:    MatcherTypeGlob,
			wantExpr: "foo",
		},
		{
			name:     "glob quoted",
			in:       "glob(`foo`)",
			wantT:    MatcherTypeGlob,
			wantExpr: "foo",
		},
		{
			name:     "regex",
			in:       "regex(^[A-Z]+$)",
			wantT:    MatcherTypeRegex,
			wantExpr: "^[A-Z]+$",
		},
		{
			name:     "regex quoted",
			in:       "regex(`^[A-Z]+$`)",
			wantT:    MatcherTypeRegex,
			wantExpr: "^[A-Z]+$",
		},
		{
			name:     "quoted expr",
			in:       "glob(`'foo'`)",
			wantT:    MatcherTypeGlob,
			wantExpr: "'foo'",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			typ, expr, err := ExtractExpr(tt.in)
			expect.NoError(t, err)
			expect.Equal(t, tt.wantT, typ)
			expect.Equal(t, tt.wantExpr, expr)
		})
	}
}

func TestExtractExprInvalid(t *testing.T) {
	tests := []struct {
		name    string
		in      string
		wantErr string
	}{
		{
			name:    "missing closing quote",
			in:      "glob(`foo)",
			wantErr: "unterminated quotes",
		},
		{
			name:    "missing closing bracket",
			in:      "glob(`foo",
			wantErr: "unterminated brackets",
		},
		{
			name:    "invalid matcher type",
			in:      "invalid(`foo`)",
			wantErr: "invalid matcher type: invalid",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, _, err := ExtractExpr(tt.in)
			expect.HasError(t, err)
			expect.ErrorContains(t, err, tt.wantErr)
		})
	}
}
