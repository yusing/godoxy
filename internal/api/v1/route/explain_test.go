package routeApi

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	entrypoint "github.com/yusing/godoxy/internal/entrypoint"
	"github.com/yusing/godoxy/internal/routing"
	"github.com/yusing/goutils/task"
)

func TestExplainHandler(t *testing.T) {
	gin.SetMode(gin.TestMode)
	testTask := task.GetTestTask(t)
	entrypoint.SetCtx(testTask, entrypoint.NewEntrypoint(testTask, nil))

	serve := func(query string) *httptest.ResponseRecorder {
		rec := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(rec)
		c.Request = httptest.NewRequestWithContext(testTask.Context(), http.MethodGet, "/api/v1/route/explain?"+query, nil)
		Explain(c)
		return rec
	}

	require.Equal(t, http.StatusBadRequest, serve("host=app.example.com").Code, "ip is required")
	require.Equal(t, http.StatusBadRequest, serve("ip=nope&host=app.example.com").Code)
	require.Equal(t, http.StatusBadRequest, serve("ip=10.1.2.3&host=app.example.com&header=no-colon").Code)
	require.Equal(t, http.StatusBadRequest, serve("ip=10.1.2.3&host=app%20example.com").Code, "malformed host must not panic")
	require.Equal(t, http.StatusBadRequest, serve("ip=10.1.2.3&host=app.example.com&method=BAD%20METHOD").Code)
	require.Equal(t, http.StatusBadRequest, serve("ip=10.1.2.3&host=app.example.com%2Finjected").Code)
	require.Equal(t, http.StatusBadRequest, serve("ip=10.1.2.3&host=app.example.com&header=Bad%20Name:value").Code)
	require.Equal(t, http.StatusBadRequest, serve("ip=10.1.2.3&host=app.example.com&header=Good:value%0D%0AInjected:yes").Code)

	rec := serve("ip=10.1.2.3&host=app.example.com&header=CF-Connecting-IP:%20198.51.100.10")
	require.Equal(t, http.StatusOK, rec.Code)
	var res entrypoint.ExplainResult
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &res))
	require.Equal(t, entrypoint.ExplainNotFound, res.Decision)
}

type failingExplainEntrypoint struct {
	routing.Entrypoint
	err error
}

func (ep failingExplainEntrypoint) Explain(entrypoint.ExplainRequest) (*entrypoint.ExplainResult, error) {
	return nil, ep.err
}

func TestExplainHandlerErrorStatus(t *testing.T) {
	for _, tc := range []struct {
		err  error
		want int
	}{{entrypoint.ErrInvalidExplainRequest, 400}, {errors.New("route overlay failed"), 500}} {
		parent := task.GetTestTask(t)
		entrypoint.SetCtx(parent, failingExplainEntrypoint{err: tc.err})
		rec := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(rec)
		c.Request = httptest.NewRequestWithContext(parent.Context(), "GET", "/api/v1/route/explain?ip=127.0.0.1&host=app.example.com", nil)
		Explain(c)
		require.Equal(t, tc.want, rec.Code)
	}
}
