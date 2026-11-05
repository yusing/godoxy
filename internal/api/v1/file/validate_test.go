package fileapi_test

import (
	"encoding/json/v2"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	nettypes "github.com/yusing/godoxy/internal/net/types"
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

func TestValidateRejectsUndefinedAddressSets(t *testing.T) {
	nettypes.PublishAddressSets(nil)
	t.Cleanup(func() { nettypes.PublishAddressSets(nil) })
	r := newFileContentRouter()
	for fileType, doc := range map[string]string{
		"middleware": "guard:\n  - use: cidr_whitelist\n    allow: [set:family]\n",
		"provider":   "app:\n  middlewares:\n    cidr_whitelist:\n      allow: [set:family]\n",
	} {
		t.Run(fileType, func(t *testing.T) {
			validate := func() int {
				req := httptest.NewRequest(http.MethodPost, "/api/v1/file/validate?type="+fileType, strings.NewReader(doc))
				w := httptest.NewRecorder()
				r.ServeHTTP(w, req)
				return w.Code
			}
			nettypes.PublishAddressSets(nil)
			require.Equal(t, http.StatusExpectationFailed, validate())
			nettypes.PublishAddressSets(map[string][]*nettypes.CIDR{"family": {}})
			require.Equal(t, http.StatusOK, validate())
		})
	}
}
