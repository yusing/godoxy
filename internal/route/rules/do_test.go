package rules

import (
	"testing"

	. "github.com/yusing/go-proxy/internal/utils/testing"
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
			name:    "serve_non_exist_path",
			input:   "serve /non-exist-path",
			wantErr: ErrInvalidArguments,
		},
		{
			name:    "serve_too_many_args",
			input:   "serve / / /",
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
			name:    "proxy_valid",
			input:   "proxy http://localhost:8080",
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
				ExpectError(t, tt.wantErr, err)
			} else {
				ExpectNoError(t, err)
			}
		})
	}
}
