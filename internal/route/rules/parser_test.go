package rules

import (
	"strconv"
	"testing"

	"github.com/yusing/go-proxy/internal/gperr"
	. "github.com/yusing/go-proxy/internal/utils/testing"
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
			name:    "invalid_escape",
			input:   `foo \bar`,
			wantErr: ErrUnsupportedEscapeChar,
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
				ExpectError(t, tt.wantErr, err)
				return
			}
			// t.Log(subject, args, err)
			ExpectNoError(t, err)
			ExpectEqual(t, subject, tt.subject)
			ExpectEqual(t, len(args), len(tt.args))
			for i, arg := range args {
				ExpectEqual(t, arg, tt.args[i])
			}
		})
	}
	t.Run("unterminated quotes", func(t *testing.T) {
		tests := []string{
			`error 403 "Forbidden 'foo' 'bar'`,
			`error 403 "Forbidden 'foo 'bar'`,
			`error 403 "Forbidden foo "bar'"`,
		}
		for i, test := range tests {
			t.Run(strconv.Itoa(i), func(t *testing.T) {
				_, _, err := parse(test)
				ExpectError(t, ErrUnterminatedQuotes, err)
			})
		}
	})
}

func BenchmarkParser(b *testing.B) {
	const input = `error 403 "Forbidden "foo" "bar""\ baz`
	for range b.N {
		_, _, err := parse(input)
		if err != nil {
			b.Fatal(err)
		}
	}
}
