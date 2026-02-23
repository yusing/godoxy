package rules

import (
	"strconv"
	"testing"

	gperr "github.com/yusing/goutils/errs"
	expect "github.com/yusing/goutils/testing"
)

func TestParser(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		subject string
		args    []string
		wantErr gperr.Error
	}{
		{
			name:    "basic",
			input:   "rewrite / /foo/bar",
			subject: "rewrite",
			args:    []string{"/", "/foo/bar"},
		},
		{
			name:    "with quotes",
			input:   `error 403 "Forbidden 'foo' 'bar'."`,
			subject: "error",
			args:    []string{"403", "Forbidden 'foo' 'bar'."},
		},
		{
			name:    "with quotes 2",
			input:   `basic_auth "username" "password"`,
			subject: "basic_auth",
			args:    []string{"username", "password"},
		},
		{
			name:    "with escaped",
			input:   `foo bar\ baz bar\r\n\tbaz bar\'\"baz`,
			subject: "foo",
			args:    []string{"bar baz", "bar\r\n\tbaz", `bar'"baz`},
		},
		{
			name:    "empty string",
			input:   `foo '' ""`,
			subject: "foo",
			args:    []string{"", ""},
		},
		{
			name:    "regex_escaped",
			input:   `foo regex(\b\B\s\S\w\W\d\D\$\.\(\)\{\}\|\?\"\')`,
			subject: "foo",
			args:    []string{`regex(\b\B\s\S\w\W\d\D\$\.\(\)\{\}\|\?"')`},
		},
		{
			name:    "quote inside argument",
			input:   `foo "abc 'def'"`,
			subject: "foo",
			args:    []string{"abc 'def'"},
		},
		{
			name:    "quote inside function",
			input:   `foo glob("'/**/to/path'")`,
			subject: "foo",
			args:    []string{"glob(\"'/**/to/path'\")"},
		},
		{
			name:    "quote inside quoted function",
			input:   "foo 'glob(\"`/**/to/path`\")'",
			subject: "foo",
			args:    []string{"glob(\"`/**/to/path`\")"},
		},
		{
			name:    "complex_regex",
			input:   `path !regex("^(_next/static|_next/image|favicon.ico).*$")`,
			subject: "path",
			args:    []string{`!regex("^(_next/static|_next/image|favicon.ico).*$")`},
		},
		{
			name:    "chaos",
			input:   `error 403 "Forbidden "foo" "bar""`,
			subject: "error",
			args:    []string{"403", "Forbidden ", "foo", " ", "bar", ""},
		},
		{
			name:    "chaos2",
			input:   `foo "'bar' 'baz'" abc\ 'foo "bar"'.`,
			subject: "foo",
			args:    []string{"'bar' 'baz'", "abc ", `foo "bar"`, "."},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			subject, args, err := parse(tt.input)
			if tt.wantErr != nil {
				expect.ErrorIs(t, tt.wantErr, err)
				return
			}
			// t.Log(subject, args, err)
			expect.NoError(t, err)
			expect.Equal(t, subject, tt.subject)
			expect.Equal(t, args, tt.args)
		})
	}
	t.Run("env substitution", func(t *testing.T) {
		// Set up test environment variables
		t.Setenv("CLOUDFLARE_API_KEY", "test-api-key-123")
		t.Setenv("DOMAIN", "example.com")

		tests := []struct {
			name    string
			input   string
			subject string
			args    []string
			wantErr string
		}{
			{
				name:    "simple env var",
				input:   `error 403 "Forbidden: ${CLOUDFLARE_API_KEY}"`,
				subject: "error",
				args:    []string{"403", "Forbidden: test-api-key-123"},
			},
			{
				name:    "multiple env vars",
				input:   `forward https://${DOMAIN}/api`,
				subject: "forward",
				args:    []string{"https://example.com/api"},
			},
			{
				name:    "env var with other text",
				input:   `auth "user-${DOMAIN}-admin" "password"`,
				subject: "auth",
				args:    []string{"user-example.com-admin", "password"},
			},
			{
				name:    "non-existent env var",
				input:   `error 404 "${NON_EXISTENT}"`,
				wantErr: ErrEnvVarNotFound.Error(),
			},
			{
				name:    "escaped",
				input:   `error 404 "$${NON_EXISTENT}"`,
				subject: "error",
				args:    []string{"404", "${NON_EXISTENT}"},
			},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				subject, args, err := parse(tt.input)
				if tt.wantErr != "" {
					expect.ErrorContains(t, err, tt.wantErr)
					return
				}
				expect.NoError(t, err)
				expect.Equal(t, subject, tt.subject)
				expect.Equal(t, args, tt.args)
			})
		}
	})
	t.Run("unterminated quotes", func(t *testing.T) {
		tests := []string{
			`error 403 "Forbidden 'foo' 'bar'`,
			`error 403 "Forbidden 'foo 'bar'`,
			`error 403 "Forbidden foo "bar'"`,
		}
		for i, test := range tests {
			t.Run(strconv.Itoa(i), func(t *testing.T) {
				_, _, err := parse(test)
				expect.ErrorIs(t, ErrUnterminatedQuotes, err)
			})
		}
	})

	t.Run("negated", func(t *testing.T) {
		test := `!error 403 "Forbidden"`
		subject, args, err := parse(test)
		expect.NoError(t, err)
		expect.Equal(t, subject, "!error")
		expect.Equal(t, args, []string{"403", "Forbidden"})
	})
}

func TestFullParse(t *testing.T) {
	input := `
- name: login page
  on: path /login
  do: pass
- name: require auth
  on: path !regex("^(_next/static|_next/image|favicon.ico).*$")
  do: require_auth
- name: redirect to login
  on: status 401 | status 403
  do: proxy /login
- name: proxy to backend
  on: path glob("/api/v1/*")
  do: proxy http://localhost:8999/
- name: proxy to backend (old /auth)
  on: path glob("/auth/*")
  do: proxy http://localhost:8999/api/v1/`

	var rules Rules
	err := parseRules(input, &rules)
	expect.NoError(t, err)
	expect.Equal(t, len(rules), 5)
	expect.Equal(t, rules[0].Name, "login page")
	expect.Equal(t, rules[0].On.String(), "path /login")
	expect.Equal(t, rules[0].Do.String(), "pass")
	expect.Equal(t, rules[1].Name, "require auth")
	expect.Equal(t, rules[1].On.String(), `path !regex("^(_next/static|_next/image|favicon.ico).*$")`)
	expect.Equal(t, rules[1].Do.String(), "require_auth")
	expect.Equal(t, rules[2].Name, "redirect to login")
	expect.Equal(t, rules[2].On.String(), "status 401 | status 403")
	expect.Equal(t, rules[2].Do.String(), "proxy /login")
	expect.Equal(t, rules[3].Name, "proxy to backend")
	expect.Equal(t, rules[3].On.String(), `path glob("/api/v1/*")`)
	expect.Equal(t, rules[3].Do.String(), "proxy http://localhost:8999/")
	expect.Equal(t, rules[4].Name, "proxy to backend (old /auth)")
	expect.Equal(t, rules[4].On.String(), `path glob("/auth/*")`)
	expect.Equal(t, rules[4].Do.String(), "proxy http://localhost:8999/api/v1/")
}

func BenchmarkParser(b *testing.B) {
	const input = `error 403 "Forbidden "foo" "bar""\ baz`
	for b.Loop() {
		_, _, err := parse(input)
		if err != nil {
			b.Fatal(err)
		}
	}
}
