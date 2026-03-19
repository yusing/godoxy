package fileapi_test

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	api "github.com/yusing/godoxy/internal/api"
	fileapi "github.com/yusing/godoxy/internal/api/v1/file"
	"github.com/yusing/goutils/fs"
)

func TestGet_PathTraversalBlocked(t *testing.T) {
	gin.SetMode(gin.TestMode)

	files, err := fs.ListFiles("..", 1, false)
	require.NoError(t, err)

	require.Greater(t, len(files), 0, "no files found")

	relativePath := files[0]

	fileContent, err := os.ReadFile(relativePath)
	require.NoError(t, err)

	r := gin.New()
	r.Use(api.ErrorHandler())
	r.GET("/api/v1/file/content", fileapi.Get)

	tests := []struct {
		name         string
		filename     string
		queryEscaped bool
	}{
		{
			name:     "dotdot_traversal",
			filename: relativePath,
		},
		{
			name:         "url_encoded_dotdot_traversal",
			filename:     relativePath,
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
			assert.NotEqual(t, fileContent, w.Body.String())
		})
	}
}
