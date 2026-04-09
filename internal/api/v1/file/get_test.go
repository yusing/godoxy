package fileapi_test

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	api "github.com/yusing/godoxy/internal/api"
	fileapi "github.com/yusing/godoxy/internal/api/v1/file"
)

func setupFileAPITestRoot(t *testing.T) string {
	t.Helper()

	oldWD, err := os.Getwd()
	require.NoError(t, err)

	root := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(root, "config", "middlewares"), 0o755))
	require.NoError(t, os.Chdir(root))

	t.Cleanup(func() {
		require.NoError(t, os.Chdir(oldWD))
	})

	return root
}

func newFileContentRouter() *gin.Engine {
	gin.SetMode(gin.TestMode)

	r := gin.New()
	r.Use(api.ErrorHandler())
	r.GET("/api/v1/file/content", fileapi.Get)
	r.PUT("/api/v1/file/content", fileapi.Set)
	return r
}

func TestGet_PathTraversalBlocked(t *testing.T) {
	root := setupFileAPITestRoot(t)

	const (
		insideFilename = "providers.yml"
		insideContent  = "app: inside\n"
		outsideContent = "app: outside\n"
	)

	require.NoError(t, os.WriteFile(filepath.Join(root, "config", insideFilename), []byte(insideContent), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(root, "secret.yml"), []byte(outsideContent), 0o644))

	r := newFileContentRouter()

	t.Run("read_in_root_file", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/file/content?type=config&filename="+insideFilename, nil)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)
		assert.Equal(t, insideContent, w.Body.String())
	})

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

			url := "/api/v1/file/content?type=config&filename=" + filename
			req := httptest.NewRequest(http.MethodGet, url, nil)
			w := httptest.NewRecorder()
			r.ServeHTTP(w, req)

			// "Blocked" means we should never successfully read the outside file.
			assert.NotEqual(t, http.StatusOK, w.Code)
			assert.NotEqual(t, outsideContent, w.Body.String())
		})
	}
}
