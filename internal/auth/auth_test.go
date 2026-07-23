package auth

import (
	"net/http"
	"net/http/httptest"
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

func (allowAuthProvider) CheckToken(*http.Request) error                  { return nil }
func (allowAuthProvider) LoginHandler(http.ResponseWriter, *http.Request) {}
func (allowAuthProvider) PostAuthCallbackHandler(http.ResponseWriter, *http.Request) {
}
func (allowAuthProvider) LogoutHandler(http.ResponseWriter, *http.Request) {}

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
