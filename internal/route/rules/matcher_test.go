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
			name:     "regex complex",
			in:       `regex("^(_next/static|_next/image|favicon.ico).*$")`,
			wantT:    MatcherTypeRegex,
			wantExpr: "^(_next/static|_next/image|favicon.ico).*$",
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

func TestNegated(t *testing.T) {
	tests := []struct {
		name string
		expr string
		in   string
		want bool
	}{
		{
			name: "negated_string_match",
			expr: "!string(`foo`)",
			in:   "foo",
			want: false,
		},
		{
			name: "negated_string_no_match",
			expr: "!string(`foo`)",
			in:   "bar",
			want: true,
		},
		{
			name: "negated_glob_match",
			expr: "!glob(`foo`)",
			in:   "foo",
			want: false,
		},
		{
			name: "negated_glob_no_match",
			expr: "!glob(`foo`)",
			in:   "bar",
			want: true,
		},
		{
			name: "negated_regex_match",
			expr: "!regex(`^(_next/static|_next/image|favicon.ico).*$`)",
			in:   "favicon.ico",
			want: false,
		},
		{
			name: "negated_regex_no_match",
			expr: "!regex(`^(_next/static|_next/image|favicon.ico).*$`)",
			in:   "bar",
			want: true,
		},
		{
			name: "negated_regex_no_match2",
			expr: "!regex(`^(_next/static|_next/image|favicon.ico).*$`)",
			in:   "/",
			want: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			matcher, err := ParseMatcher(tt.expr)
			expect.NoError(t, err)
			expect.Equal(t, tt.want, matcher(tt.in))
		})
	}
}
