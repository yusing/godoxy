package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/yusing/godoxy/internal/auth"
	"github.com/yusing/godoxy/internal/common"
	"github.com/yusing/godoxy/internal/route/rules"
	rulepresets "github.com/yusing/godoxy/internal/route/rules/presets"
)

func TestRegisteredHandlerServesEmbeddedWebUIAPI(t *testing.T) {
	preserveRuleHandlers(t)
	_ = newAuthenticatedHandler(t)

	prevAuthHandler := rules.GetAuthHandler()
	rules.InitAuthHandler(auth.AuthOrProceed)
	t.Cleanup(func() {
		rules.InitAuthHandler(prevAuthHandler)
	})

	require.NoError(t, RegisterHandlers())
	handler := embeddedWebUIHandler(t)

	validRequest := newJSONRequest(t, http.MethodPost, "/api/v1/auth/callback", map[string]string{
		"username": common.APIUser,
		"password": common.APIPassword,
	})
	validRequest.Host = "app.example.com"
	validRequest.Header.Set("Origin", "https://app.example.com")
	validRecorder := httptest.NewRecorder()
	handler.ServeHTTP(validRecorder, validRequest)
	require.Equal(t, http.StatusOK, validRecorder.Code)
	sessionCookie := findCookie(validRecorder.Result().Cookies(), "godoxy_token")
	require.NotNil(t, sessionCookie)

	tests := []struct {
		name       string
		request    func() *http.Request
		wantStatus int
		wantSource string
	}{
		{
			name: "invalid credentials",
			request: func() *http.Request {
				req := newJSONRequest(t, http.MethodPost, "/api/v1/auth/callback", map[string]string{
					"username": common.APIUser,
					"password": "incorrect",
				})
				req.Host = "app.example.com"
				req.Header.Set("Origin", "https://app.example.com")
				return req
			},
			wantStatus: http.StatusBadRequest,
		},
		{
			name: "cross-origin login",
			request: func() *http.Request {
				req := newJSONRequest(t, http.MethodPost, "/api/v1/auth/callback", map[string]string{
					"username": common.APIUser,
					"password": common.APIPassword,
				})
				req.Host = "app.example.com"
				req.Header.Set("Origin", "https://evil.example.com")
				return req
			},
			wantStatus: http.StatusForbidden,
		},
		{
			name: "malformed login",
			request: func() *http.Request {
				req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/callback", strings.NewReader("{"))
				req.Host = "app.example.com"
				req.Header.Set("Content-Type", "application/json")
				req.Header.Set("Origin", "https://app.example.com")
				return req
			},
			wantStatus: http.StatusBadRequest,
		},
		{
			name: "unrelated static path",
			request: func() *http.Request {
				return httptest.NewRequest(http.MethodGet, "/future.js", nil)
			},
			wantStatus: http.StatusTeapot,
			wantSource: "fallback",
		},
		{
			name: "public version route",
			request: func() *http.Request {
				return httptest.NewRequest(http.MethodGet, "/api/v1/version", nil)
			},
			wantStatus: http.StatusOK,
		},
		{
			name: "unknown future API route",
			request: func() *http.Request {
				req := httptest.NewRequest(http.MethodGet, "/api/v1/future", nil)
				req.AddCookie(sessionCookie)
				return req
			},
			wantStatus: http.StatusNotFound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			handler.ServeHTTP(recorder, tt.request())
			assert.Equal(t, tt.wantStatus, recorder.Code)
			assert.Equal(t, tt.wantSource, recorder.Header().Get("X-Response-Source"))
		})
	}
}

func TestRegisteredAndStandaloneHandlersHaveMatchingAuthPolicy(t *testing.T) {
	preserveRuleHandlers(t)
	_ = newAuthenticatedHandler(t)
	require.NoError(t, RegisterHandlers())

	registered, ok := rules.GetHandler("api")
	require.True(t, ok)
	standalone := NewHandler(true)

	request := func() *http.Request {
		req := newJSONRequest(t, http.MethodPost, "/api/v1/auth/callback", map[string]string{
			"username": common.APIUser,
			"password": common.APIPassword,
		})
		req.Host = "app.example.com"
		req.Header.Set("Origin", "https://evil.example.com")
		return req
	}

	registeredRecorder := httptest.NewRecorder()
	registered.ServeHTTP(registeredRecorder, request())
	standaloneRecorder := httptest.NewRecorder()
	standalone.ServeHTTP(standaloneRecorder, request())

	assert.Equal(t, standaloneRecorder.Code, registeredRecorder.Code)
	assert.Equal(t, http.StatusForbidden, registeredRecorder.Code)
	assert.Equal(t, standaloneRecorder.Body.String(), registeredRecorder.Body.String())
}

func TestRegisterHandlersPreservesNameCollisions(t *testing.T) {
	for _, collision := range []string{"api", "local_api"} {
		t.Run(collision, func(t *testing.T) {
			preserveRuleHandlers(t)
			sentinel := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusAccepted)
			})
			require.True(t, rules.RegisterHandler(collision, sentinel))

			err := RegisterHandlers()
			require.ErrorContains(t, err, collision)

			registered, ok := rules.GetHandler(collision)
			require.True(t, ok)
			recorder := httptest.NewRecorder()
			registered.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/", nil))
			assert.Equal(t, http.StatusAccepted, recorder.Code)

			other := "api"
			if collision == other {
				other = "local_api"
			}
			_, ok = rules.GetHandler(other)
			assert.False(t, ok)
		})
	}
}

func TestRegisterHandlersWithDisabledAuthOmitsAuthRoutes(t *testing.T) {
	preserveRuleHandlers(t)

	prevSecret := common.APIJWTSecret
	prevDisableAuth := common.DebugDisableAuth
	prevIssuerURL := common.OIDCIssuerURL
	common.APIJWTSecret = nil
	common.DebugDisableAuth = true
	common.OIDCIssuerURL = ""
	t.Cleanup(func() {
		common.APIJWTSecret = prevSecret
		common.DebugDisableAuth = prevDisableAuth
		common.OIDCIssuerURL = prevIssuerURL
	})

	require.NoError(t, auth.Initialize())
	require.NoError(t, RegisterHandlers())
	registered, ok := rules.GetHandler("api")
	require.True(t, ok)

	callbackRecorder := httptest.NewRecorder()
	registered.ServeHTTP(
		callbackRecorder,
		httptest.NewRequest(http.MethodPost, "/api/v1/auth/callback", nil),
	)
	assert.Equal(t, http.StatusNotFound, callbackRecorder.Code)

	versionRecorder := httptest.NewRecorder()
	registered.ServeHTTP(
		versionRecorder,
		httptest.NewRequest(http.MethodGet, "/api/v1/version", nil),
	)
	assert.Equal(t, http.StatusOK, versionRecorder.Code)
}

func embeddedWebUIHandler(t *testing.T) http.Handler {
	t.Helper()
	webUIRules, ok := rulepresets.GetRulePreset("webui.yml")
	require.True(t, ok)
	fallback := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("X-Response-Source", "fallback")
		w.WriteHeader(http.StatusTeapot)
	})
	return webUIRules.BuildHandler(fallback.ServeHTTP)
}

func preserveRuleHandlers(t *testing.T) {
	t.Helper()
	prevAPI, hadAPI := rules.GetHandler("api")
	prevLocalAPI, hadLocalAPI := rules.GetHandler("local_api")
	rules.ReplaceHandler("api", nil)
	rules.ReplaceHandler("local_api", nil)
	t.Cleanup(func() {
		if hadAPI {
			rules.ReplaceHandler("api", prevAPI)
		} else {
			rules.ReplaceHandler("api", nil)
		}
		if hadLocalAPI {
			rules.ReplaceHandler("local_api", prevLocalAPI)
		} else {
			rules.ReplaceHandler("local_api", nil)
		}
	})
}
