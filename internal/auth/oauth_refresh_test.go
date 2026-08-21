package auth

import (
	"encoding/json"
	"maps"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/oauth2"
)

func TestDoRefreshTokenValidatesRefreshedIdentity(t *testing.T) {
	provider := setupProvider(t)

	tests := []struct {
		name         string
		claims       map[string]any
		allowedUsers []string
		wantErr      error
		wantUsername string
		wantGroups   []string
	}{
		{
			name: "uses refreshed identity",
			claims: map[string]any{
				"preferred_username": "refreshed-user",
				"groups":             []string{"refreshed-group"},
			},
			allowedUsers: []string{"refreshed-user"},
			wantUsername: "refreshed-user",
			wantGroups:   []string{"refreshed-group"},
		},
		{
			name:         "rejects missing application claims",
			claims:       map[string]any{},
			allowedUsers: []string{"old-user"},
			wantErr:      ErrRefreshTokenFailure,
		},
		{
			name: "rejects identity that is no longer allowed",
			claims: map[string]any{
				"preferred_username": "blocked-user",
			},
			allowedUsers: []string{"old-user"},
			wantErr:      ErrUserNotAllowed,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			claims := map[string]any{
				"iss": provider.server.URL,
				"aud": clientID,
				"exp": time.Now().Add(time.Hour).Unix(),
			}
			maps.Copy(claims, tt.claims)
			signedIDToken := provider.SignClaims(t, claims)
			tokenResponse, err := json.Marshal(map[string]any{
				"access_token":  "refreshed-access-token",
				"token_type":    "Bearer",
				"refresh_token": "replacement-refresh-token",
				"expires_in":    3600,
				"id_token":      signedIDToken,
			})
			require.NoError(t, err)
			tokenServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write(tokenResponse)
			}))
			t.Cleanup(tokenServer.Close)

			auth := &OIDCProvider{
				oauthConfig: &oauth2.Config{
					ClientID: clientID,
					Endpoint: oauth2.Endpoint{TokenURL: tokenServer.URL},
				},
				oidcVerifier: provider.verifier,
				allowedUsers: tt.allowedUsers,
			}
			oldSession := newSession("old-user", []string{"old-group"})
			refreshToken := &oauthRefreshToken{
				Username:     oldSession.Username,
				RefreshToken: "old-refresh-token",
				Expiry:       time.Now().Add(time.Hour),
			}

			result, err := auth.doRefreshToken(t.Context(), refreshToken, &oldSession)

			if tt.wantErr != nil {
				require.ErrorIs(t, err, tt.wantErr)
				assert.Nil(t, result)
				assert.Nil(t, refreshToken.result)
				return
			}
			require.NoError(t, err)
			require.NotNil(t, result)
			assert.Equal(t, tt.wantUsername, result.newSession.Username)
			assert.Equal(t, tt.wantGroups, result.newSession.Groups)

			stored, ok := oauthRefreshTokens.Load(string(result.newSession.SessionID))
			require.True(t, ok)
			assert.Equal(t, tt.wantUsername, stored.Username)
			assert.Equal(t, "replacement-refresh-token", stored.RefreshToken)
			oauthRefreshTokens.Delete(string(result.newSession.SessionID))
		})
	}
}
