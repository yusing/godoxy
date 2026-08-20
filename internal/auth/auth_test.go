package auth

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/yusing/godoxy/internal/common"
)

func TestValidateUserPassCredentials(t *testing.T) {
	preserveAuthConfig(t)

	tests := []struct {
		name        string
		username    string
		password    string
		disableAuth bool
		oidcIssuer  string
		wantMissing bool
	}{
		{name: "credentials configured", username: "admin", password: "secret"},
		{name: "username missing", password: "secret", wantMissing: true},
		{name: "password missing", username: "admin", wantMissing: true},
		{name: "both credentials missing", wantMissing: true},
		{name: "authentication explicitly disabled", disableAuth: true},
		{name: "OIDC enabled", oidcIssuer: "https://id.example.com"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			common.APIUser = tt.username
			common.APIPassword = tt.password
			common.DebugDisableAuth = tt.disableAuth
			common.OIDCIssuerURL = tt.oidcIssuer

			err := validateUserPassCredentials()
			if tt.wantMissing {
				assert.ErrorIs(t, err, errMissingUserPassCredentials)
				return
			}
			require.NoError(t, err)
		})
	}
}

func TestInitializeRejectsMissingUserPassCredentials(t *testing.T) {
	preserveAuthConfig(t)

	common.APIUser = ""
	common.APIPassword = ""
	common.DebugDisableAuth = false
	common.OIDCIssuerURL = ""

	assert.ErrorIs(t, Initialize(t.Context()), errMissingUserPassCredentials)
}

func TestAuthOrProceedFailsClosedWhileEnabledAuthenticationInitializes(t *testing.T) {
	preserveAuthConfig(t)

	common.DebugDisableAuth = false
	common.APIJWTSecret = []byte("configured")
	common.OIDCIssuerURL = ""
	setDefaultAuth(nil)
	recorder := httptest.NewRecorder()

	proceed := AuthOrProceed(recorder, httptest.NewRequest(http.MethodGet, "/", nil))

	assert.False(t, proceed)
	assert.Equal(t, http.StatusServiceUnavailable, recorder.Code)
	assert.Contains(t, recorder.Body.String(), "authentication is initializing")
}

func TestAuthOrProceedAllowsRequestsWhenAuthenticationIsDisabled(t *testing.T) {
	preserveAuthConfig(t)

	common.DebugDisableAuth = true
	setDefaultAuth(nil)
	recorder := httptest.NewRecorder()

	proceed := AuthOrProceed(recorder, httptest.NewRequest(http.MethodGet, "/", nil))

	assert.True(t, proceed)
	assert.Equal(t, http.StatusOK, recorder.Code)
}

func TestAuthCheckHandlesRefreshedSession(t *testing.T) {
	preserveAuthConfig(t)
	setDefaultAuth(refreshAuthProvider{})
	recorder := httptest.NewRecorder()

	AuthCheckHandler(recorder, httptest.NewRequest(http.MethodHead, "/api/v1/auth/check", nil))

	assert.Equal(t, http.StatusOK, recorder.Code)
	assert.Empty(t, recorder.Header().Get("Location"))
}

func TestAuthOrProceedMapsRefreshedSessionByRequestType(t *testing.T) {
	preserveAuthConfig(t)
	setDefaultAuth(refreshAuthProvider{})

	tests := []struct {
		name         string
		request      func() *http.Request
		wantStatus   int
		wantLocation string
	}{
		{
			name: "document GET redirects to the original URI",
			request: func() *http.Request {
				req := httptest.NewRequest(http.MethodGet, "/routes?section=apps", nil)
				req.Header.Set("Accept", "text/html")
				return req
			},
			wantStatus:   http.StatusFound,
			wantLocation: "/routes?section=apps",
		},
		{
			name: "JSON GET is rejected",
			request: func() *http.Request {
				req := httptest.NewRequest(http.MethodGet, "/api/v1/health", nil)
				req.Header.Set("Accept", "application/json")
				return req
			},
			wantStatus: http.StatusForbidden,
		},
		{
			name: "POST is rejected without replay",
			request: func() *http.Request {
				req := httptest.NewRequest(http.MethodPost, "/api/v1/route/playground", nil)
				req.Header.Set("Accept", "text/html")
				return req
			},
			wantStatus: http.StatusForbidden,
		},
		{
			name: "WebSocket upgrade is rejected",
			request: func() *http.Request {
				req := httptest.NewRequest(http.MethodGet, "/api/v1/health", nil)
				req.Header.Set("Accept", "text/html")
				req.Header.Set("Connection", "Upgrade")
				req.Header.Set("Upgrade", "websocket")
				return req
			},
			wantStatus: http.StatusForbidden,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			recorder := httptest.NewRecorder()

			proceed := AuthOrProceed(recorder, tt.request())

			assert.False(t, proceed)
			assert.Equal(t, tt.wantStatus, recorder.Code)
			assert.Equal(t, tt.wantLocation, recorder.Header().Get("Location"))
		})
	}
}

func TestRedirectToRequestURIRemainsLocal(t *testing.T) {
	tests := []struct {
		name         string
		requestURL   *url.URL
		wantLocation string
	}{
		{
			name:         "path and repeated query",
			requestURL:   &url.URL{Path: "/routes", RawQuery: "tag=a&tag=b%20c"},
			wantLocation: "/routes?tag=a&tag=b%20c",
		},
		{
			name:         "explicit empty query",
			requestURL:   &url.URL{Path: "/routes", ForceQuery: true},
			wantLocation: "/routes?",
		},
		{
			name:         "escaped path",
			requestURL:   &url.URL{Path: "/apps/name", RawPath: "/apps%2Fname"},
			wantLocation: "/apps%2Fname",
		},
		{
			name:         "network path",
			requestURL:   &url.URL{Path: "//attacker.example/path", RawQuery: "next=/routes"},
			wantLocation: "/",
		},
		{
			name:         "absolute request URL",
			requestURL:   &url.URL{Scheme: "https", Host: "attacker.example", Path: "/routes", RawQuery: "x=1"},
			wantLocation: "/routes?x=1",
		},
		{
			name:         "opaque request URL",
			requestURL:   &url.URL{Scheme: "https", Opaque: "//attacker.example/routes"},
			wantLocation: "/",
		},
		{
			name:         "empty path",
			requestURL:   &url.URL{},
			wantLocation: "/",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/", nil)
			req.URL = tt.requestURL
			recorder := httptest.NewRecorder()

			RedirectToRequestURI(recorder, req)

			assert.Equal(t, http.StatusFound, recorder.Code)
			assert.Equal(t, tt.wantLocation, recorder.Header().Get("Location"))
		})
	}
}

func TestAuthenticationProviderPublicationIsConcurrentSafe(t *testing.T) {
	preserveAuthConfig(t)

	common.DebugDisableAuth = false
	common.APIJWTSecret = []byte("configured")
	common.OIDCIssuerURL = ""
	setDefaultAuth(nil)
	provider := allowAuthProvider{}

	var wg sync.WaitGroup
	wg.Go(func() {
		for range 1_000 {
			recorder := httptest.NewRecorder()
			AuthOrProceed(recorder, httptest.NewRequest(http.MethodGet, "/", nil))
			assert.Contains(t, []int{http.StatusOK, http.StatusServiceUnavailable}, recorder.Code)
		}
	})
	for i := range 1_000 {
		if i%2 == 0 {
			setDefaultAuth(provider)
		} else {
			setDefaultAuth(nil)
		}
	}
	wg.Wait()
}

type allowAuthProvider struct{}

func (allowAuthProvider) CheckToken(*http.Request) error { return nil }
func (allowAuthProvider) LoginHandler(http.ResponseWriter, *http.Request) LoginResult {
	return LoginResponseHandled
}
func (allowAuthProvider) PostAuthCallbackHandler(http.ResponseWriter, *http.Request) {
}
func (allowAuthProvider) LogoutHandler(http.ResponseWriter, *http.Request) {}

type refreshAuthProvider struct{}

func (refreshAuthProvider) CheckToken(*http.Request) error { return errors.New("refresh required") }
func (refreshAuthProvider) LoginHandler(http.ResponseWriter, *http.Request) LoginResult {
	return LoginSessionRefreshed
}
func (refreshAuthProvider) PostAuthCallbackHandler(http.ResponseWriter, *http.Request) {}
func (refreshAuthProvider) LogoutHandler(http.ResponseWriter, *http.Request)           {}

func preserveAuthConfig(t *testing.T) {
	t.Helper()
	previousUser := common.APIUser
	previousPassword := common.APIPassword
	previousDisableAuth := common.DebugDisableAuth
	previousIssuerURL := common.OIDCIssuerURL
	previousJWTSecret := common.APIJWTSecret
	previousDefaultAuth := GetDefaultAuth()
	t.Cleanup(func() {
		common.APIUser = previousUser
		common.APIPassword = previousPassword
		common.DebugDisableAuth = previousDisableAuth
		common.OIDCIssuerURL = previousIssuerURL
		common.APIJWTSecret = previousJWTSecret
		setDefaultAuth(previousDefaultAuth)
	})
}
