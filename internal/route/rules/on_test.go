package rules

import (
	"encoding/base64"
	"fmt"
	"net/http"
	"net/url"
	"testing"

	"github.com/yusing/go-proxy/internal/gperr"
	. "github.com/yusing/go-proxy/internal/utils/testing"
	"golang.org/x/crypto/bcrypt"
)

func TestParseOn(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr gperr.Error
	}{
		// header
		{
			name:    "header_valid_kv",
			input:   "header Connection Upgrade",
			wantErr: nil,
		},
		{
			name:    "header_valid_k",
			input:   "header Connection",
			wantErr: nil,
		},
		{
			name:    "header_missing_arg",
			input:   "header",
			wantErr: ErrExpectKVOptionalV,
		},
		// query
		{
			name:    "query_valid_kv",
			input:   "query key value",
			wantErr: nil,
		},
		{
			name:    "query_valid_k",
			input:   "query key",
			wantErr: nil,
		},
		{
			name:    "query_missing_arg",
			input:   "query",
			wantErr: ErrExpectKVOptionalV,
		},
		{
			name:    "cookie_valid_kv",
			input:   "cookie key value",
			wantErr: nil,
		},
		{
			name:    "cookie_valid_k",
			input:   "cookie key",
			wantErr: nil,
		},
		{
			name:    "cookie_missing_arg",
			input:   "cookie",
			wantErr: ErrExpectKVOptionalV,
		},
		// method
		{
			name:    "method_valid",
			input:   "method GET",
			wantErr: nil,
		},
		{
			name:    "method_invalid",
			input:   "method invalid",
			wantErr: ErrInvalidArguments,
		},
		{
			name:    "method_missing_arg",
			input:   "method",
			wantErr: ErrExpectOneArg,
		},
		// path
		{
			name:    "path_valid",
			input:   "path /home",
			wantErr: nil,
		},
		{
			name:    "path_missing_arg",
			input:   "path",
			wantErr: ErrExpectOneArg,
		},
		// remote
		{
			name:    "remote_valid",
			input:   "remote 127.0.0.1",
			wantErr: nil,
		},
		{
			name:    "remote_invalid",
			input:   "remote abcd",
			wantErr: ErrInvalidArguments,
		},
		{
			name:    "remote_missing_arg",
			input:   "remote",
			wantErr: ErrExpectOneArg,
		},
		{
			name:    "unknown_target",
			input:   "unknown",
			wantErr: ErrInvalidOnTarget,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			on := &RuleOn{}
			err := on.Parse(tt.input)
			if tt.wantErr != nil {
				ExpectError(t, tt.wantErr, err)
			} else {
				ExpectNoError(t, err)
			}
		})
	}
}

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
			checker: "basic_auth user " + string(Must(bcrypt.GenerateFromPassword([]byte("password"), bcrypt.DefaultCost))),
			input: &http.Request{
				Header: http.Header{
					"Authorization": {"Basic " + base64.StdEncoding.EncodeToString([]byte("user:password"))}, // "user:password"
				},
			},
			want: true,
		},
		{
			name:    "basic_auth_incorrect",
			checker: "basic_auth user " + string(Must(bcrypt.GenerateFromPassword([]byte("password"), bcrypt.DefaultCost))),
			input: &http.Request{
				Header: http.Header{
					"Authorization": {"Basic " + base64.StdEncoding.EncodeToString([]byte("user:incorrect"))}, // "user:wrong"
				},
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
			on, err := parseOn(tt.checker)
			ExpectNoError(t, err)
			got := on.Check(Cache{}, tt.input)
			if tt.want != got {
				t.Errorf("want %v, got %v", tt.want, got)
			}
		})
	}
}
