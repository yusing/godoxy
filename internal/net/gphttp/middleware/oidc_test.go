package middleware

import (
	"testing"

	. "github.com/yusing/go-proxy/internal/utils/testing"
)

func TestOIDCMiddlewarePerRouteConfig(t *testing.T) {
	t.Run("middleware struct has correct fields", func(t *testing.T) {
		middleware := &oidcMiddleware{
			AllowedUsers:  []string{"custom-user"},
			AllowedGroups: []string{"custom-group"},
			ClientID:      "custom-client-id",
			ClientSecret:  "custom-client-secret",
			Scopes:        "openid,profile,email,groups",
		}

		ExpectEqual(t, middleware.AllowedUsers, []string{"custom-user"})
		ExpectEqual(t, middleware.AllowedGroups, []string{"custom-group"})
		ExpectEqual(t, middleware.ClientID, "custom-client-id")
		ExpectEqual(t, middleware.ClientSecret, "custom-client-secret")
		ExpectEqual(t, middleware.Scopes, "openid,profile,email,groups")
	})

	t.Run("middleware struct handles empty values", func(t *testing.T) {
		middleware := &oidcMiddleware{}

		ExpectEqual(t, middleware.AllowedUsers, nil)
		ExpectEqual(t, middleware.AllowedGroups, nil)
		ExpectEqual(t, middleware.ClientID, "")
		ExpectEqual(t, middleware.ClientSecret, "")
		ExpectEqual(t, middleware.Scopes, "")
	})
}
