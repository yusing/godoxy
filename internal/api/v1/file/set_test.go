package fileapi_test

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const validProviderYAML = `app:
  host: attacker.com
  port: 443
  scheme: https
`

func TestSet_PathTraversalBlocked(t *testing.T) {
	root := setupFileAPITestRoot(t)
	r := newFileContentRouter()

	t.Run("write_in_root_file", func(t *testing.T) {
		req := httptest.NewRequest(
			http.MethodPut,
			"/api/v1/file/content?type=provider&filename=providers.yml",
			strings.NewReader(validProviderYAML),
		)
		req.Header.Set("Content-Type", "text/plain")

		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)

		content, err := os.ReadFile(filepath.Join(root, "config", "providers.yml"))
		require.NoError(t, err)
		assert.Equal(t, validProviderYAML, string(content))
	})

	const originalContent = "do not overwrite\n"
	require.NoError(t, os.WriteFile(filepath.Join(root, "secret.yml"), []byte(originalContent), 0o644))

	tests := []struct {
		name         string
		filename     string
		queryEscaped bool
	}{
		{
			name:     "dotdot_traversal_to_sibling_file",
			filename: "../secret.yml",
		},
		{
			name:         "url_encoded_dotdot_traversal_to_sibling_file",
			filename:     "../secret.yml",
			queryEscaped: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			filename := tt.filename
			if tt.queryEscaped {
				filename = url.QueryEscape(filename)
			}

			req := httptest.NewRequest(
				http.MethodPut,
				"/api/v1/file/content?type=provider&filename="+filename,
				strings.NewReader(validProviderYAML),
			)
			req.Header.Set("Content-Type", "text/plain")

			w := httptest.NewRecorder()
			r.ServeHTTP(w, req)

			assert.NotEqual(t, http.StatusOK, w.Code)

			content, err := os.ReadFile(filepath.Join(root, "secret.yml"))
			require.NoError(t, err)
			assert.Equal(t, originalContent, string(content))
		})
	}
}
