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
			name:     "regex with parentheses",
			in:       "regex(test(group))",
			wantT:    MatcherTypeRegex,
			wantExpr: "test(group)",
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

func TestGlobMatcherNoSeparators(t *testing.T) {
	tests := []struct {
		name    string
		pattern string
		input   string
		want    bool
	}{
		{name: "star_matches_slash", pattern: "/api/*", input: "/api/users/123", want: true},
		{name: "star_matches_one_segment", pattern: "/api/*", input: "/api/users", want: true},
		{name: "star_requires_slash_after_prefix", pattern: "/api/*", input: "/api", want: false},
		{name: "double_star_matches_nested", pattern: "/static/**", input: "/static/css/style.css", want: true},
		{name: "question_matches_slash", pattern: "/file?.txt", input: "/file/.txt", want: true},
		{name: "question_one_rune", pattern: "/file?.txt", input: "/file1.txt", want: true},
		{name: "question_rejects_two_runes", pattern: "/file?.txt", input: "/file12.txt", want: false},
		{name: "range_digit", pattern: "/v[0-9]/api", input: "/v1/api", want: true},
		{name: "range_digit_rejects_two", pattern: "/v[0-9]/api", input: "/v10/api", want: false},
		{name: "set_listed", pattern: "/file[123].txt", input: "/file1.txt", want: true},
		{name: "set_rejects_other", pattern: "/file[123].txt", input: "/file4.txt", want: false},
		{name: "negated_set", pattern: "/file[!123].txt", input: "/fileA.txt", want: true},
		{name: "alternatives", pattern: "/api/{users,posts,comments}", input: "/api/users", want: true},
		{name: "alternatives_reject_nested", pattern: "/api/{users,posts,comments}", input: "/api/users/123", want: false},
		{name: "hex_hyphen_set", pattern: "/users/[-abcdef0123456789]*/profile", input: "/users/abc-def/profile", want: true},
		{name: "assets_alt", pattern: "/static/**/*.{css,js}", input: "/static/css/style.css", want: true},
		{name: "user_agent_slash", pattern: "Mozilla*", input: "Mozilla/5.0", want: true},
		{name: "single_alt_brace", pattern: "{ab*}", input: "abc", want: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m, err := GlobMatcher(tt.pattern, false)
			expect.NoError(t, err)
			expect.Equal(t, tt.want, m(tt.input))
		})
	}
}

func TestGlobMatcherRejectsMixedClass(t *testing.T) {
	for _, pattern := range []string{
		"/users/[a-f0-9-]*/profile",
		"/users/[a-f0-9]*/profile",
	} {
		t.Run(pattern, func(t *testing.T) {
			_, err := GlobMatcher(pattern, false)
			expect.HasError(t, err)
			expect.ErrorContains(t, err, "syntax error")
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
