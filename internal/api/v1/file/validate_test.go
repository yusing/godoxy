package fileapi_test

import (
	"encoding/json/v2"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestValidate_MiddlewareErrorHintIsJSON(t *testing.T) {
	r := newFileContentRouter()
	req := httptest.NewRequest(
		http.MethodPost,
		"/api/v1/file/validate?type=middleware",
		strings.NewReader("test:\n  - use: RealIP\n    heder: X-Real-IP\n"),
	)

	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusExpectationFailed, w.Code)
	var response any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &response))
	assert.Contains(t, w.Body.String(), "Do you mean Header?")
}
