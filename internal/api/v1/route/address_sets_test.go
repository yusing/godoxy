package routeApi

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	nettypes "github.com/yusing/godoxy/internal/net/types"
)

func TestValidateRejectsUndefinedAddressSets(t *testing.T) {
	nettypes.PublishAddressSets(nil)
	t.Cleanup(func() { nettypes.PublishAddressSets(nil) })
	r := gin.New()
	r.POST("/validate", Validate)
	validate := func() int {
		req := httptest.NewRequest(http.MethodPost, "/validate", strings.NewReader("middlewares:\n  cidr_whitelist:\n    allow: [set:family]\n"))
		req.Header.Set("Content-Type", "application/yaml")
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		return w.Code
	}
	require.Equal(t, http.StatusExpectationFailed, validate())
	nettypes.PublishAddressSets(map[string][]*nettypes.CIDR{"family": {}})
	require.Equal(t, http.StatusOK, validate())
}

func TestPlaygroundRejectsUndefinedAddressSets(t *testing.T) {
	nettypes.PublishAddressSets(nil)
	t.Cleanup(func() { nettypes.PublishAddressSets(nil) })
	request := PlaygroundRequest{Rules: "remote set:family { error 403 Forbidden }"}
	response := runPlayground(t, request)
	require.ErrorIs(t, response.ExecutionError, nettypes.ErrUndefinedAddressSet)
	require.False(t, response.UpstreamCalled)
	require.ErrorIs(t, response.ParsedRules[0].ValidationError, nettypes.ErrUndefinedAddressSet)

	nettypes.PublishAddressSets(map[string][]*nettypes.CIDR{"family": {}})
	response = runPlayground(t, request)
	require.NoError(t, response.ExecutionError)
	require.True(t, response.UpstreamCalled)
}
