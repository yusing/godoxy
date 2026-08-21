package accesslog

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yusing/godoxy/internal/net/gphttp"
)

type recordingAccessLogger struct {
	AccessLogger
	requests int
	errors   int
}

func (l *recordingAccessLogger) LogRequest(*http.Request, *http.Response) {
	l.requests++
}

func (l *recordingAccessLogger) LogError(*http.Request, error) {
	l.errors++
}

func TestNonUserRequestFilteredLogger(t *testing.T) {
	base := &recordingAccessLogger{}
	logger := &nonUserRequestFilteredLogger{AccessLogger: base}
	response := &http.Response{StatusCode: http.StatusOK}
	testErr := errors.New("test error")

	normalRequest := httptest.NewRequest(http.MethodGet, "http://example.com/", nil)
	logger.LogRequest(normalRequest, response)
	logger.LogError(normalRequest, testErr)

	internalRequest := httptest.NewRequest(http.MethodGet, "http://example.com/", nil)
	internalRequest = internalRequest.WithContext(gphttp.WithNonUserRequest(internalRequest.Context()))
	logger.LogRequest(internalRequest, response)
	logger.LogError(internalRequest, testErr)

	require.Equal(t, 1, base.requests)
	require.Equal(t, 1, base.errors)
}
