package auth

import (
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

	assert.ErrorIs(t, Initialize(), errMissingUserPassCredentials)
}

func preserveAuthConfig(t *testing.T) {
	t.Helper()
	previousUser := common.APIUser
	previousPassword := common.APIPassword
	previousDisableAuth := common.DebugDisableAuth
	previousIssuerURL := common.OIDCIssuerURL
	t.Cleanup(func() {
		common.APIUser = previousUser
		common.APIPassword = previousPassword
		common.DebugDisableAuth = previousDisableAuth
		common.OIDCIssuerURL = previousIssuerURL
	})
}
