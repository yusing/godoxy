package rules_test

import (
	"encoding/base64"
	"fmt"
	"net/http"
	"net/url"
	"testing"

	"github.com/yusing/godoxy/internal/route"
	"github.com/yusing/godoxy/internal/route/routes"
	. "github.com/yusing/godoxy/internal/route/rules"
	expect "github.com/yusing/goutils/testing"
	"golang.org/x/crypto/bcrypt"
)

type testCorrectness struct {
	name    string
	checker string
	input   *http.Request
	want    bool
}

func genCorrectnessTestCases(field string, genRequest func(k, v string) *http.Request) []testCorrectness {
	return []testCorrectness{
		{
			name:    field + "_match",
			checker: field + " foo bar",
			input:   genRequest("foo", "bar"),
			want:    true,
		},
		{
			name:    field + "_no_match",
			checker: field + " foo baz",
			input:   genRequest("foo", "bar"),
			want:    false,
		},
		{
			name:    field + "_exists",
			checker: field + " foo",
			input:   genRequest("foo", "abcd"),
			want:    true,
		},
		{
			name:    field + "_not_exists",
			checker: field + " foo",
			input:   genRequest("bar", "abcd"),
			want:    false,
		},
	}
}

func TestOnCorrectness(t *testing.T) {
	tests := []testCorrectness{
		{
			name:    "method_match",
			checker: "method GET",
			input:   &http.Request{Method: http.MethodGet},
			want:    true,
		},
		{
			name:    "method_no_match",
			checker: "method GET",
			input:   &http.Request{Method: http.MethodPost},
			want:    false,
		},
		{
			name:    "path_exact_match",
			checker: "path /example",
			input: &http.Request{
				URL: &url.URL{Path: "/example"},
			},
			want: true,
		},
		{
			name:    "path_wildcard_match",
			checker: "path /example/*",
			input: &http.Request{
				URL: &url.URL{Path: "/example/123"},
			},
			want: true,
		},
		{
			name:    "remote_match",
			checker: "remote 192.168.1.0/24",
			input: &http.Request{
				RemoteAddr: "192.168.1.5",
			},
			want: true,
		},
		{
			name:    "remote_no_match",
			checker: "remote 192.168.1.0/24",
			input: &http.Request{
				RemoteAddr: "192.168.2.5",
			},
			want: false,
		},
		{
			name:    "basic_auth_correct",
			checker: "basic_auth user " + string(expect.Must(bcrypt.GenerateFromPassword([]byte("password"), bcrypt.DefaultCost))),
			input: &http.Request{
				Header: http.Header{
					"Authorization": {"Basic " + base64.StdEncoding.EncodeToString([]byte("user:password"))}, // "user:password"
				},
			},
			want: true,
		},
		{
			name:    "basic_auth_incorrect",
			checker: "basic_auth user " + string(expect.Must(bcrypt.GenerateFromPassword([]byte("password"), bcrypt.DefaultCost))),
			input: &http.Request{
				Header: http.Header{
					"Authorization": {"Basic " + base64.StdEncoding.EncodeToString([]byte("user:incorrect"))}, // "user:wrong"
				},
			},
			want: false,
		},
		{
			name:    "route_match",
			checker: "route example",
			input: routes.WithRouteContext(&http.Request{}, expect.Must(route.NewFileServer(&route.Route{
				Alias: "example",
				Root:  "/",
			}))),
			want: true,
		},
		{
			name:    "route_no_match",
			checker: "route example",
			input: &http.Request{
				Header: http.Header{},
			},
			want: false,
		},
	}

	tests = append(tests, genCorrectnessTestCases("header", func(k, v string) *http.Request {
		return &http.Request{
			Header: http.Header{k: []string{v}},
		}
	})...)
	tests = append(tests, genCorrectnessTestCases("query", func(k, v string) *http.Request {
		return &http.Request{
			URL: &url.URL{
				RawQuery: fmt.Sprintf("%s=%s", k, v),
			},
		}
	})...)
	tests = append(tests, genCorrectnessTestCases("cookie", func(k, v string) *http.Request {
		return &http.Request{
			Header: http.Header{
				"Cookie": {fmt.Sprintf("%s=%s", k, v)},
			},
		}
	})...)
	tests = append(tests, genCorrectnessTestCases("form", func(k, v string) *http.Request {
		return &http.Request{
			Form: url.Values{
				k: []string{v},
			},
		}
	})...)
	tests = append(tests, genCorrectnessTestCases("postform", func(k, v string) *http.Request {
		return &http.Request{
			PostForm: url.Values{
				k: []string{v},
			},
		}
	})...)

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var on RuleOn
			err := on.Parse(tt.checker)
			expect.NoError(t, err)
			got := on.Check(Cache{}, tt.input)
			expect.Equal(t, tt.want, got)
		})
	}
}
