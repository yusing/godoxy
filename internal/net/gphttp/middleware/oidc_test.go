package middleware

import (
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yusing/godoxy/internal/auth"
	"github.com/yusing/godoxy/internal/common"
	expect "github.com/yusing/goutils/testing"
)

func TestOIDCMiddlewarePerRouteConfig(t *testing.T) {
	t.Run("middleware struct has correct fields", func(t *testing.T) {
		middleware := &oidcMiddleware{
			AllowedUsers:  []string{"custom-user"},
			AllowedGroups: []string{"custom-group"},
			ClientID:      "custom-client-id",
			ClientSecret:  "custom-client-secret",
			Scopes:        strings.Split("openid,profile,email,groups", ","),
		}

		expect.Equal(t, middleware.AllowedUsers, []string{"custom-user"})
		expect.Equal(t, middleware.AllowedGroups, []string{"custom-group"})
		expect.Equal(t, middleware.ClientID, "custom-client-id")
		expect.Equal(t, middleware.ClientSecret, "custom-client-secret")
		expect.Equal(t, middleware.Scopes, strings.Split("openid,profile,email,groups", ","))
	})

	t.Run("middleware struct handles empty values", func(t *testing.T) {
		middleware := &oidcMiddleware{}

		expect.Equal(t, middleware.AllowedUsers, nil)
		expect.Equal(t, middleware.AllowedGroups, nil)
		expect.Equal(t, middleware.ClientID, "")
		expect.Equal(t, middleware.ClientSecret, "")
		expect.Empty(t, middleware.Scopes)
	})
}

func TestOIDCMiddlewareRetriesAfterInitFailure(t *testing.T) {
	previousIssuerURL := common.OIDCIssuerURL
	previousAllowedUsers := common.OIDCAllowedUsers
	previousAllowedGroups := common.OIDCAllowedGroups
	t.Cleanup(func() {
		common.OIDCIssuerURL = previousIssuerURL
		common.OIDCAllowedUsers = previousAllowedUsers
		common.OIDCAllowedGroups = previousAllowedGroups
	})

	common.OIDCIssuerURL = "http://127.0.0.1:1"
	common.OIDCAllowedUsers = []string{"user"}
	common.OIDCAllowedGroups = nil

	middleware := &oidcMiddleware{}
	req := httptest.NewRequest(http.MethodGet, "/", nil)

	t.Run("first call", func(t *testing.T) {
		w := httptest.NewRecorder()
		require.False(t, middleware.before(w, req))
		require.Equal(t, http.StatusInternalServerError, w.Code)
		require.Nil(t, middleware.auth)
		require.Equal(t, int32(0), middleware.isInitialized)
	})

	t.Run("retry call", func(t *testing.T) {
		w := httptest.NewRecorder()
		var panicValue any
		func() {
			defer func() {
				panicValue = recover()
			}()
			require.False(t, middleware.before(w, req))
		}()
		require.Nil(t, panicValue, "middleware.before panicked after prior init failure")
		require.Equal(t, http.StatusInternalServerError, w.Code)
		require.Nil(t, middleware.auth)
		require.Equal(t, int32(0), middleware.isInitialized)
	})
}

func TestShouldHandleOIDCLogin(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{name: "missing token", err: auth.ErrMissingOAuthToken, want: true},
		{name: "invalid token", err: auth.ErrInvalidOAuthToken, want: true},
		{name: "wrapped invalid token", err: fmt.Errorf("expired: %w", auth.ErrInvalidOAuthToken), want: true},
		{name: "user not allowed", err: auth.ErrUserNotAllowed, want: false},
		{name: "unrelated error", err: errors.New("provider failure"), want: false},
		{name: "nil", err: nil, want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, shouldHandleOIDCLogin(tt.err))
		})
	}
}
