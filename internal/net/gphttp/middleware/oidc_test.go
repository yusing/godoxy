package middleware

import (
	"testing"

	expect "github.com/yusing/goutils/testing"
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

		expect.Equal(t, middleware.AllowedUsers, []string{"custom-user"})
		expect.Equal(t, middleware.AllowedGroups, []string{"custom-group"})
		expect.Equal(t, middleware.ClientID, "custom-client-id")
		expect.Equal(t, middleware.ClientSecret, "custom-client-secret")
		expect.Equal(t, middleware.Scopes, "openid,profile,email,groups")
	})

	t.Run("middleware struct handles empty values", func(t *testing.T) {
		middleware := &oidcMiddleware{}

		expect.Equal(t, middleware.AllowedUsers, nil)
		expect.Equal(t, middleware.AllowedGroups, nil)
		expect.Equal(t, middleware.ClientID, "")
		expect.Equal(t, middleware.ClientSecret, "")
		expect.Equal(t, middleware.Scopes, "")
	})
}
