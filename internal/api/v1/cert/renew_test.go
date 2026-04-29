package certapi

import (
	"context"
	"crypto/tls"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	autocert "github.com/yusing/godoxy/internal/autocert/types"
	"github.com/yusing/goutils/task"
)

type stubProvider struct {
	started bool
	waited  bool
}

func (p *stubProvider) GetCert(*tls.ClientHelloInfo) (*tls.Certificate, error) { return nil, nil }
func (p *stubProvider) GetCertInfos() ([]autocert.CertInfo, error)             { return nil, nil }
func (p *stubProvider) ScheduleRenewalAll(task.Parent)                         {}
func (p *stubProvider) ObtainCertAll() error                                   { return nil }
func (p *stubProvider) ForceExpiryAll() bool                                   { p.started = true; return true }
func (p *stubProvider) WaitRenewalDone(context.Context) bool                   { p.waited = true; return true }

func TestRenewWithoutWebsocketStillTriggersProvider(t *testing.T) {
	gin.SetMode(gin.TestMode)
	provider := &stubProvider{}
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/api/v1/cert/renew", nil)
	c.Request = c.Request.WithContext(context.WithValue(c.Request.Context(), autocert.ContextKey{}, provider))

	Renew(c)

	require.True(t, provider.started)
}
