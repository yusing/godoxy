package api

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/yusing/godoxy/internal/auth"
	autocert "github.com/yusing/godoxy/internal/autocert/types"
	"github.com/yusing/godoxy/internal/common"
	"github.com/yusing/goutils/task"
)

func TestAuthCheckIssuesCSRFCookie(t *testing.T) {
	handler := newAuthenticatedHandler(t)

	req := httptest.NewRequest(http.MethodHead, "/api/v1/auth/check", nil)
	req.Host = "app.example.com"
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusFound, rec.Code)

	csrfCookie := findCookie(rec.Result().Cookies(), auth.CSRFCookieName)
	require.NotNil(t, csrfCookie)
	assert.NotEmpty(t, csrfCookie.Value)
	assert.Empty(t, csrfCookie.Domain)
	assert.Equal(t, "/", csrfCookie.Path)
	assert.Equal(t, http.SameSiteStrictMode, csrfCookie.SameSite)
}

func TestUserPassCallbackAllowsSameOriginFormPostWithoutCSRFCookie(t *testing.T) {
	handler := newAuthenticatedHandler(t)

	req := newJSONRequest(t, http.MethodPost, "/api/v1/auth/callback", map[string]string{
		"username": common.APIUser,
		"password": common.APIPassword,
	})
	req.Host = "app.example.com"
	req.Header.Set("Origin", "https://app.example.com")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
	tokenCookie := findCookie(rec.Result().Cookies(), "godoxy_token")
	require.NotNil(t, tokenCookie)
	assert.NotEmpty(t, tokenCookie.Value)
	csrfCookie := findCookie(rec.Result().Cookies(), auth.CSRFCookieName)
	require.NotNil(t, csrfCookie)
	assert.NotEmpty(t, csrfCookie.Value)
}

func TestUserPassCallbackRejectsCrossOriginPostWithoutCSRFCookie(t *testing.T) {
	handler := newAuthenticatedHandler(t)

	req := newJSONRequest(t, http.MethodPost, "/api/v1/auth/callback", map[string]string{
		"username": common.APIUser,
		"password": common.APIPassword,
	})
	req.Host = "app.example.com"
	req.Header.Set("Origin", "https://evil.example.com")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusForbidden, rec.Code)
	csrfCookie := findCookie(rec.Result().Cookies(), auth.CSRFCookieName)
	require.NotNil(t, csrfCookie)
	assert.NotEmpty(t, csrfCookie.Value)
}

func TestUserPassCallbackAcceptsValidCSRFCookie(t *testing.T) {
	handler := newAuthenticatedHandler(t)
	csrfCookie := issueCSRFCookie(t, handler)

	req := newJSONRequest(t, http.MethodPost, "/api/v1/auth/callback", map[string]string{
		"username": common.APIUser,
		"password": common.APIPassword,
	})
	req.Host = "app.example.com"
	req.AddCookie(csrfCookie)
	req.Header.Set(auth.CSRFHeaderName, csrfCookie.Value)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
	tokenCookie := findCookie(rec.Result().Cookies(), "godoxy_token")
	require.NotNil(t, tokenCookie)
	assert.NotEmpty(t, tokenCookie.Value)
}

func TestUnsafeRequestAcceptsQuotedCSRFCookieValue(t *testing.T) {
	handler := newAuthenticatedHandler(t)
	csrfCookie := issueCSRFCookie(t, handler)
	sessionToken := issueSessionToken(t)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/logout", nil)
	req.Host = "app.example.com"
	req.Header.Set("Cookie", `godoxy_token=`+sessionToken+`; godoxy_csrf="`+csrfCookie.Value+`"`)
	req.Header.Set(auth.CSRFHeaderName, csrfCookie.Value)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusFound, rec.Code)
}

func TestLogoutRequiresCSRFCookie(t *testing.T) {
	handler := newAuthenticatedHandler(t)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/logout", nil)
	req.Host = "app.example.com"
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusForbidden, rec.Code)
}

func TestLoginAllowsSameOriginPostWithoutCSRFCookie(t *testing.T) {
	handler := newAuthenticatedHandler(t)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", nil)
	req.Host = "app.example.com"
	req.Header.Set("Origin", "https://app.example.com")
	req.Header.Set("Accept", "text/html")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusFound, rec.Code)
	csrfCookie := findCookie(rec.Result().Cookies(), auth.CSRFCookieName)
	require.NotNil(t, csrfCookie)
	assert.NotEmpty(t, csrfCookie.Value)
}

func TestGetLogoutRouteStillAvailableForFrontend(t *testing.T) {
	handler := newAuthenticatedHandler(t)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/auth/logout", nil)
	req.Host = "app.example.com"
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusFound, rec.Code)
}

func TestCertRenewRejectsCrossOriginWebSocketRequest(t *testing.T) {
	handler := newAuthenticatedHandler(t)
	provider := &stubAutocertProvider{}
	sessionToken := issueSessionToken(t)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/cert/renew", nil)
	req.Host = "app.example.com"
	req.Header.Set("Connection", "Upgrade")
	req.Header.Set("Upgrade", "websocket")
	req.Header.Set("Sec-WebSocket-Version", "13")
	req.Header.Set("Sec-WebSocket-Key", "dGhlIHNhbXBsZSBub25jZQ==")
	req.Header.Set("Origin", "https://evil.example.com")
	req.AddCookie(&http.Cookie{Name: "godoxy_token", Value: sessionToken})
	req = req.WithContext(context.WithValue(req.Context(), autocert.ContextKey{}, provider))

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusForbidden, rec.Code)
	assert.Zero(t, provider.forceExpiryCalls)
}

func newAuthenticatedHandler(t *testing.T) *gin.Engine {
	t.Helper()

	gin.SetMode(gin.TestMode)

	prevSecret := common.APIJWTSecret
	prevUser := common.APIUser
	prevPassword := common.APIPassword
	prevDisableAuth := common.DebugDisableAuth
	prevIssuerURL := common.OIDCIssuerURL
	prevSkipOriginCheck := common.APISkipOriginCheck

	common.APIJWTSecret = []byte("0123456789abcdef0123456789abcdef")
	common.APIUser = "username"
	common.APIPassword = "password"
	common.DebugDisableAuth = false
	common.OIDCIssuerURL = ""
	common.APISkipOriginCheck = false

	t.Cleanup(func() {
		common.APIJWTSecret = prevSecret
		common.APIUser = prevUser
		common.APIPassword = prevPassword
		common.DebugDisableAuth = prevDisableAuth
		common.OIDCIssuerURL = prevIssuerURL
		common.APISkipOriginCheck = prevSkipOriginCheck
	})

	require.NoError(t, auth.Initialize())
	return NewHandler(true)
}

func issueCSRFCookie(t *testing.T, handler http.Handler) *http.Cookie {
	t.Helper()

	req := httptest.NewRequest(http.MethodHead, "/api/v1/auth/check", nil)
	req.Host = "app.example.com"
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	csrfCookie := findCookie(rec.Result().Cookies(), auth.CSRFCookieName)
	require.NotNil(t, csrfCookie)
	return csrfCookie
}

func issueSessionToken(t *testing.T) string {
	t.Helper()

	userpass, ok := auth.GetDefaultAuth().(*auth.UserPassAuth)
	require.True(t, ok)

	token, err := userpass.NewToken()
	require.NoError(t, err)
	return token
}

func newJSONRequest(t *testing.T, method, target string, body any) *http.Request {
	t.Helper()

	encoded, err := json.Marshal(body)
	require.NoError(t, err)

	req := httptest.NewRequest(method, target, bytes.NewReader(encoded))
	req.Header.Set("Content-Type", "application/json")
	return req
}

func findCookie(cookies []*http.Cookie, name string) *http.Cookie {
	for _, cookie := range cookies {
		if cookie.Name == name {
			return cookie
		}
	}
	return nil
}

type stubAutocertProvider struct {
	forceExpiryCalls int
}

func (p *stubAutocertProvider) GetCert(*tls.ClientHelloInfo) (*tls.Certificate, error) {
	return nil, nil
}

func (p *stubAutocertProvider) GetCertInfos() ([]autocert.CertInfo, error) {
	return nil, nil
}

func (p *stubAutocertProvider) ScheduleRenewalAll(task.Parent) {}

func (p *stubAutocertProvider) ObtainCertAll() error {
	return nil
}

func (p *stubAutocertProvider) ForceExpiryAll() bool {
	p.forceExpiryCalls++
	return true
}

func (p *stubAutocertProvider) WaitRenewalDone(context.Context) bool {
	return true
}
