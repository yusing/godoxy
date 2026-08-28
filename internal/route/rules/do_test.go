package rules

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/stretchr/testify/require"
	expect "github.com/yusing/goutils/testing"
)

func TestParseCommands(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr error
	}{
		// bypass tests
		{
			name:    "bypass_valid",
			input:   "bypass",
			wantErr: nil,
		},
		{
			name:    "bypass_invalid_with_args",
			input:   "bypass /",
			wantErr: ErrInvalidArguments,
		},
		// rewrite tests
		{
			name:    "rewrite_valid",
			input:   "rewrite / /foo/bar",
			wantErr: nil,
		},
		{
			name:    "rewrite_missing_target",
			input:   "rewrite /",
			wantErr: ErrInvalidArguments,
		},
		{
			name:    "rewrite_too_many_args",
			input:   "rewrite / / /",
			wantErr: ErrInvalidArguments,
		},
		// serve tests
		{
			name:    "serve_valid",
			input:   "serve /",
			wantErr: nil,
		},
		{
			name:    "serve_missing_path",
			input:   "serve ",
			wantErr: ErrInvalidArguments,
		},
		{
			name:    "serve_file_missing_path",
			input:   "serve_file ",
			wantErr: ErrInvalidArguments,
		},
		{
			name:    "serve_non_exist_path",
			input:   "serve /non-exist-path",
			wantErr: ErrInvalidArguments,
		},
		{
			name:    "serve_file_non_exist_path",
			input:   "serve_file /non-exist-path",
			wantErr: ErrInvalidArguments,
		},
		{
			name:    "serve_too_many_args",
			input:   "serve / / /",
			wantErr: ErrInvalidArguments,
		},
		{
			name:    "serve_file_too_many_args",
			input:   "serve_file / / /",
			wantErr: ErrInvalidArguments,
		},
		// handle tests
		{
			name:    "handle_valid",
			input:   "handle api",
			wantErr: nil,
		},
		{
			name:    "handle_missing_name",
			input:   "handle",
			wantErr: ErrInvalidArguments,
		},
		{
			name:    "handle_too_many_args",
			input:   "handle api extra",
			wantErr: ErrInvalidArguments,
		},
		// redirect tests
		{
			name:    "redirect_valid",
			input:   "redirect /",
			wantErr: nil,
		},
		{
			name:    "redirect_too_many_args",
			input:   "redirect / /",
			wantErr: ErrInvalidArguments,
		},
		// error directive tests
		{
			name:    "error_valid",
			input:   "error 404 Not\\ Found",
			wantErr: nil,
		},
		{
			name:    "error_missing_status_code",
			input:   "error Not\\ Found",
			wantErr: ErrInvalidArguments,
		},
		{
			name:    "error_too_many_args",
			input:   "error 404 Not\\ Found extra",
			wantErr: ErrInvalidArguments,
		},
		{
			name:    "error_no_escaped_space",
			input:   "error 404 Not Found",
			wantErr: ErrInvalidArguments,
		},
		{
			name:    "error_invalid_status_code",
			input:   "error 123 abc",
			wantErr: ErrInvalidArguments,
		},
		// proxy directive tests
		{
			name:    "proxy_valid_abs",
			input:   "proxy http://localhost:8080",
			wantErr: nil,
		},
		{
			name:    "proxy_valid_rel",
			input:   "proxy /foo/bar",
			wantErr: nil,
		},
		{
			name:    "proxy_missing_target",
			input:   "proxy",
			wantErr: ErrInvalidArguments,
		},
		{
			name:    "proxy_too_many_args",
			input:   "proxy http://localhost:8080 extra",
			wantErr: ErrInvalidArguments,
		},
		{
			name:    "proxy_invalid_url",
			input:   "proxy invalid_url",
			wantErr: ErrInvalidArguments,
		},
		// unknown directive test
		{
			name:    "unknown_directive",
			input:   "unknown /",
			wantErr: ErrUnknownDirective,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd := Command{}
			err := cmd.Parse(tt.input)
			if tt.wantErr != nil {
				expect.ErrorIs(t, tt.wantErr, err)
			} else {
				expect.NoError(t, err)
			}
		})
	}
}

func TestParseCommandServeFileValid(t *testing.T) {
	f, err := os.CreateTemp(t.TempDir(), "serve-file-*.html")
	if err != nil {
		t.Fatal(err)
	}
	f.Close()

	cmd := Command{}
	err = cmd.Parse("serve_file " + f.Name())
	expect.NoError(t, err)
}

func TestParseCommandServeFileRejectsDirectory(t *testing.T) {
	cmd := Command{}
	err := cmd.Parse("serve_file " + t.TempDir())
	expect.ErrorIs(t, ErrInvalidArguments, err)
}
func TestMiddlewareCommandExecutesInDeclarationOrder(t *testing.T) {
	previousResolver := requestMiddlewareResolver
	t.Cleanup(func() {
		InitRequestMiddlewareResolver(previousResolver)
	})

	var order []string
	InitRequestMiddlewareResolver(func(name string, _ map[string]any) (RequestMiddleware, error) {
		return func(_ http.ResponseWriter, r *http.Request) bool {
			order = append(order, name)
			r.Header.Add("X-Middleware", name)
			return true
		}, nil
	})

	var configured Rules
	require.NoError(t, configured.Parse(`
default {
	middleware first
	middleware second
}
`))

	fallbackCalled := false
	handler := configured.BuildHandler(func(w http.ResponseWriter, _ *http.Request) {
		fallbackCalled = true
		order = append(order, "fallback")
		w.WriteHeader(http.StatusNoContent)
	})
	request := httptest.NewRequest(http.MethodGet, "http://unknown.example/", nil)
	response := httptest.NewRecorder()

	handler.ServeHTTP(response, request)

	require.Equal(t, http.StatusNoContent, response.Code)
	require.True(t, fallbackCalled)
	require.Equal(t, []string{"first", "second", "fallback"}, order)
	require.Equal(t, []string{"first", "second"}, request.Header.Values("X-Middleware"))
}

func TestMiddlewareCommandTerminatesWhenMiddlewareHandlesRequest(t *testing.T) {
	previousResolver := requestMiddlewareResolver
	t.Cleanup(func() {
		InitRequestMiddlewareResolver(previousResolver)
	})

	InitRequestMiddlewareResolver(func(string, map[string]any) (RequestMiddleware, error) {
		return func(w http.ResponseWriter, _ *http.Request) bool {
			http.Error(w, "blocked", http.StatusForbidden)
			return false
		}, nil
	})

	var configured Rules
	require.NoError(t, configured.Parse(`
default {
	middleware blocker
}
`))

	fallbackCalled := false
	handler := configured.BuildHandler(func(http.ResponseWriter, *http.Request) {
		fallbackCalled = true
	})
	response := httptest.NewRecorder()

	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "http://unknown.example/", nil))

	require.Equal(t, http.StatusForbidden, response.Code)
	require.False(t, fallbackCalled)
}
func TestMiddlewareCommandTerminationDoesNotFallThroughWithoutResponse(t *testing.T) {
	previousResolver := requestMiddlewareResolver
	t.Cleanup(func() {
		InitRequestMiddlewareResolver(previousResolver)
	})
	InitRequestMiddlewareResolver(func(string, map[string]any) (RequestMiddleware, error) {
		return func(http.ResponseWriter, *http.Request) bool {
			return false
		}, nil
	})

	var configured Rules
	require.NoError(t, configured.Parse(`
default {
	middleware blocker
}
`))

	fallbackCalled := false
	handler := configured.BuildHandler(func(http.ResponseWriter, *http.Request) {
		fallbackCalled = true
	})

	handler.ServeHTTP(
		httptest.NewRecorder(),
		httptest.NewRequest(http.MethodGet, "http://unknown.example/", nil),
	)

	require.False(t, fallbackCalled)
}

func TestMatchedMiddlewareTerminationDoesNotFallThroughWithoutResponse(t *testing.T) {
	previousResolver := requestMiddlewareResolver
	t.Cleanup(func() {
		InitRequestMiddlewareResolver(previousResolver)
	})
	InitRequestMiddlewareResolver(func(string, map[string]any) (RequestMiddleware, error) {
		return func(http.ResponseWriter, *http.Request) bool {
			return false
		}, nil
	})

	var configured Rules
	require.NoError(t, configured.Parse(`
path / {
	middleware blocker
}
`))

	fallbackCalled := false
	handler := configured.BuildHandler(func(http.ResponseWriter, *http.Request) {
		fallbackCalled = true
	})

	handler.ServeHTTP(
		httptest.NewRecorder(),
		httptest.NewRequest(http.MethodGet, "http://unknown.example/", nil),
	)

	require.False(t, fallbackCalled)
}

func TestMiddlewareCommandValidation(t *testing.T) {
	previousResolver := requestMiddlewareResolver
	t.Cleanup(func() {
		InitRequestMiddlewareResolver(previousResolver)
	})

	resolveErr := errors.New("middleware unavailable")
	InitRequestMiddlewareResolver(func(string, map[string]any) (RequestMiddleware, error) {
		return nil, resolveErr
	})

	tests := []struct {
		name    string
		command string
		wantErr error
	}{
		{name: "missing name", command: "middleware", wantErr: ErrInvalidArguments},
		{name: "too many names", command: "middleware first second", wantErr: ErrInvalidArguments},
		{name: "resolver error", command: "middleware unknown", wantErr: resolveErr},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var command Command
			require.ErrorIs(t, command.Parse(test.command), test.wantErr)
		})
	}
}

func TestRulesValidateRejectsMiddlewareAfterResponseMatcher(t *testing.T) {
	previousResolver := requestMiddlewareResolver
	t.Cleanup(func() {
		InitRequestMiddlewareResolver(previousResolver)
	})
	InitRequestMiddlewareResolver(func(string, map[string]any) (RequestMiddleware, error) {
		return func(http.ResponseWriter, *http.Request) bool { return true }, nil
	})

	tests := map[string]string{
		"direct": `status 404 {
			middleware test
		}`,
		"block action": `status 404 {
			middleware test {
			}
		}`,
		"nested action block": `status 404 {
			method GET {
				middleware test
			}
		}`,
		"nested response matcher": `default {
			status 404 {
				middleware test
			}
		}`,
		"mixed-phase action block": `default {
			method GET {
				middleware test
				set resp_header X-Test yes
			}
		}`,
	}
	for name, config := range tests {
		t.Run(name, func(t *testing.T) {
			var configured Rules
			require.NoError(t, configured.Parse(config))
			require.ErrorContains(t, configured.Validate(), "request middleware cannot be used in a response-phase rule or action block")
		})
	}
}

func TestMiddlewareCommandBlockProperties(t *testing.T) {
	previousResolver := requestMiddlewareResolver
	t.Cleanup(func() {
		InitRequestMiddlewareResolver(previousResolver)
	})

	var resolvedName string
	var resolvedOptions map[string]any
	InitRequestMiddlewareResolver(func(name string, options map[string]any) (RequestMiddleware, error) {
		resolvedName = name
		resolvedOptions = options
		return func(http.ResponseWriter, *http.Request) bool { return true }, nil
	})

	var configured Rules
	require.NoError(t, configured.Parse(`
default {
	middleware RealIP {
		header: X-Forwarded-For
		from:
			- 127.0.0.1/32
		recursive: true
	}
}
`))

	require.Equal(t, "RealIP", resolvedName)
	require.Equal(t, "X-Forwarded-For", resolvedOptions["header"])
	require.Equal(t, []any{"127.0.0.1/32"}, resolvedOptions["from"])
	require.Equal(t, true, resolvedOptions["recursive"])
}

func TestMiddlewareCommandBlockDoesNotRewriteQuotedTabs(t *testing.T) {
	previousResolver := requestMiddlewareResolver
	t.Cleanup(func() {
		InitRequestMiddlewareResolver(previousResolver)
	})

	var resolvedHeader any
	InitRequestMiddlewareResolver(func(_ string, options map[string]any) (RequestMiddleware, error) {
		resolvedHeader = options["header"]
		return func(http.ResponseWriter, *http.Request) bool { return true }, nil
	})

	var configured Rules
	err := configured.Parse(`
default {
	middleware RealIP {
		header: "X-Forwarded	For"
	}
}
`)
	require.NoError(t, err)
	require.Equal(t, "X-Forwarded	For", resolvedHeader)
}

func TestMiddlewareCommandBlockRequiresName(t *testing.T) {
	var configured Rules
	err := configured.Parse(`
default {
	middleware {
		header: X-Forwarded-For
	}
}
`)
	require.ErrorIs(t, err, ErrInvalidArguments)
}
