package auth

import (
	"crypto/rand"
	"crypto/rsa"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/coreos/go-oidc/v3/oidc"
	"github.com/coreos/go-oidc/v3/oidc/oidctest"
	"github.com/yusing/godoxy/internal/common"
	"golang.org/x/oauth2"

	expect "github.com/yusing/goutils/testing"
)

// setupMockOIDC configures mock OIDC provider for testing.
func setupMockOIDC(t *testing.T) {
	t.Helper()

	provider := setupProvider(t)
	setDefaultAuth(expect.Must(NewOIDCProvider(
		t.Context(),
		provider.server.URL,
		clientID,
		"test-secret",
		[]string{"test-user"},
		[]string{"test-group1", "test-group2"},
	)))
}

const (
	keyID    = "test-key-id"
	clientID = "test-client-id"
)

type provider struct {
	server   *httptest.Server
	key      *rsa.PrivateKey
	verifier *oidc.IDTokenVerifier
}

func (j *provider) SignClaims(t *testing.T, claims map[string]any) string {
	t.Helper()

	rawClaims, err := json.Marshal(claims)
	expect.NoError(t, err)
	return oidctest.SignIDToken(j.key, keyID, oidc.RS256, string(rawClaims))
}

func setupProvider(t *testing.T) *provider {
	t.Helper()

	// Generate an RSA key pair for the test.
	privKey, err := rsa.GenerateKey(rand.Reader, 2048)
	expect.NoError(t, err)

	mockServer := &oidctest.Server{
		PublicKeys: []oidctest.PublicKey{
			{
				PublicKey: privKey.Public(),
				KeyID:     keyID,
				Algorithm: oidc.RS256,
			},
		},
	}
	server := httptest.NewServer(mockServer)
	t.Cleanup(server.Close)
	mockServer.SetIssuer(server.URL)

	oidcProvider, err := oidc.NewProvider(t.Context(), server.URL)
	expect.NoError(t, err)

	return &provider{
		server: server,
		key:    privKey,
		verifier: oidcProvider.Verifier(&oidc.Config{
			ClientID: clientID,
		}),
	}
}

func cleanup() {
	setDefaultAuth(nil)
}

func TestOIDCLoginHandler(t *testing.T) {
	// Setup
	common.APIJWTSecret = []byte("test-secret")
	t.Cleanup(cleanup)
	setupMockOIDC(t)

	tests := []struct {
		name         string
		wantStatus   int
		wantRedirect bool
	}{
		{
			name:         "Success - Redirects to provider",
			wantStatus:   http.StatusFound,
			wantRedirect: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, OIDCAuthInitPath, nil)
			w := httptest.NewRecorder()

			GetDefaultAuth().(*OIDCProvider).HandleAuth(w, req)

			if got := w.Code; got != tt.wantStatus {
				t.Errorf("OIDCLoginHandler() status = %v, want %v", got, tt.wantStatus)
			}

			if tt.wantRedirect {
				if loc := w.Header().Get("Location"); loc == "" {
					t.Error("OIDCLoginHandler() missing redirect location")
				}

				cookie := w.Header().Get("Set-Cookie")
				if cookie == "" {
					t.Error("OIDCLoginHandler() missing state cookie")
				}
			}
		})
	}
}

func TestOIDCCallbackHandler(t *testing.T) {
	// Setup
	common.APIJWTSecret = []byte("test-secret")
	t.Cleanup(cleanup)
	tests := []struct {
		name       string
		state      string
		code       string
		setupMocks bool
		wantStatus int
	}{
		{
			name:       "Success - Valid callback",
			state:      "valid-state",
			code:       "valid-code",
			setupMocks: true,
			wantStatus: http.StatusFound,
		},
		{
			name:       "Failure - Missing state",
			code:       "valid-code",
			setupMocks: true,
			wantStatus: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.setupMocks {
				setupMockOIDC(t)
			}

			req := httptest.NewRequest(http.MethodGet, "/auth/callback?code="+tt.code+"&state="+tt.state, nil)
			if tt.state != "" {
				req.AddCookie(&http.Cookie{
					Name:  GetDefaultAuth().(*OIDCProvider).getAppScopedCookieName(CookieOauthState),
					Value: tt.state,
				})
			}
			w := httptest.NewRecorder()

			GetDefaultAuth().(*OIDCProvider).PostAuthCallbackHandler(w, req)

			if got := w.Code; got != tt.wantStatus {
				t.Errorf("OIDCCallbackHandler() status = %v, want %v", got, tt.wantStatus)
			}

			if tt.wantStatus == http.StatusTemporaryRedirect {
				setCookie := expect.Must(http.ParseSetCookie(w.Header().Get("Set-Cookie")))
				expect.Equal(t, setCookie.Name, CookieOauthToken)
				expect.True(t, setCookie.Value != "")
				expect.Equal(t, setCookie.Path, "/")
				expect.Equal(t, setCookie.SameSite, http.SameSiteLaxMode)
				expect.Equal(t, setCookie.HttpOnly, true)
			}
		})
	}
}

func TestInitOIDC(t *testing.T) {
	provider := setupProvider(t)

	tests := []struct {
		name          string
		issuerURL     string
		clientID      string
		clientSecret  string
		redirectURL   string
		logoutURL     string
		allowedUsers  []string
		allowedGroups []string
		wantErr       bool
	}{
		{
			name:    "Fail - Empty configuration",
			wantErr: true,
		},
		{
			name:         "Success - Valid configuration with users",
			issuerURL:    provider.server.URL,
			clientID:     "client_id",
			clientSecret: "client_secret",
			allowedUsers: []string{"user1", "user2"},
			wantErr:      false,
		},
		{
			name:          "Success - Valid configuration with groups",
			issuerURL:     provider.server.URL,
			clientID:      "client_id",
			clientSecret:  "client_secret",
			allowedGroups: []string{"group1", "group2"},
			wantErr:       false,
		},
		{
			name:          "Success - Valid configuration with users, groups and logout URL",
			issuerURL:     provider.server.URL,
			clientID:      "client_id",
			clientSecret:  "client_secret",
			logoutURL:     "https://example.com/logout",
			allowedUsers:  []string{"user1", "user2"},
			allowedGroups: []string{"group1", "group2"},
			wantErr:       false,
		},
		{
			name:         "Fail - No allowed users or allowed groups",
			issuerURL:    "https://example.com",
			clientID:     "client_id",
			clientSecret: "client_secret",
			wantErr:      true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := NewOIDCProvider(t.Context(), tt.issuerURL, tt.clientID, tt.clientSecret, tt.allowedUsers, tt.allowedGroups)
			if (err != nil) != tt.wantErr {
				t.Errorf("InitOIDC() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestCheckToken(t *testing.T) {
	provider := setupProvider(t)

	tests := []struct {
		name          string
		allowedUsers  []string
		allowedGroups []string
		claims        map[string]any
		wantErr       error
	}{
		{
			name:         "Success - Valid token with allowed user",
			allowedUsers: []string{"user1"},
			claims: map[string]any{
				"iss":                provider.server.URL,
				"aud":                clientID,
				"exp":                time.Now().Add(time.Hour).Unix(),
				"preferred_username": "user1",
				"groups":             []string{"group1"},
			},
		},
		{
			name:          "Success - Valid token with allowed group",
			allowedGroups: []string{"group1"},
			claims: map[string]any{
				"iss":                provider.server.URL,
				"aud":                clientID,
				"exp":                time.Now().Add(time.Hour).Unix(),
				"preferred_username": "user1",
				"groups":             []string{"group1"},
			},
		},
		{
			name:         "Success - Server omits groups, but user is allowed",
			allowedUsers: []string{"user1"},
			claims: map[string]any{
				"iss":                provider.server.URL,
				"aud":                clientID,
				"exp":                time.Now().Add(time.Hour).Unix(),
				"preferred_username": "user1",
			},
		},
		{
			name:          "Success - Server omits preferred_username, but group is allowed",
			allowedGroups: []string{"group1"},
			claims: map[string]any{
				"iss":    provider.server.URL,
				"aud":    clientID,
				"exp":    time.Now().Add(time.Hour).Unix(),
				"groups": []string{"group1"},
			},
		},
		{
			name:          "Success - Valid token with allowed user and group",
			allowedUsers:  []string{"user1"},
			allowedGroups: []string{"group1"},
			claims: map[string]any{
				"iss":                provider.server.URL,
				"aud":                clientID,
				"exp":                time.Now().Add(time.Hour).Unix(),
				"preferred_username": "user1",
				"groups":             []string{"group1"},
			},
		},
		{
			name:          "Error - User not allowed",
			allowedUsers:  []string{"user2", "user3"},
			allowedGroups: []string{"group2", "group3"},
			claims: map[string]any{
				"iss":                provider.server.URL,
				"aud":                clientID,
				"exp":                time.Now().Add(time.Hour).Unix(),
				"preferred_username": "user1",
				"groups":             []string{"group1"},
			},
			wantErr: ErrUserNotAllowed,
		},
		{
			name: "Error - Server returns incorrect issuer",
			claims: map[string]any{
				"iss":                "https://example.com",
				"aud":                clientID,
				"exp":                time.Now().Add(time.Hour).Unix(),
				"preferred_username": "user1",
				"groups":             []string{"group1"},
			},
			wantErr: ErrInvalidOAuthToken,
		},
		{
			name: "Error - Server returns incorrect audience",
			claims: map[string]any{
				"iss":                provider.server.URL,
				"aud":                "some-other-audience",
				"exp":                time.Now().Add(time.Hour).Unix(),
				"preferred_username": "user1",
				"groups":             []string{"group1"},
			},
			wantErr: ErrInvalidOAuthToken,
		},
		{
			name: "Error - Server returns expired token",
			claims: map[string]any{
				"iss":                provider.server.URL,
				"aud":                clientID,
				"exp":                time.Now().Add(-time.Hour).Unix(),
				"preferred_username": "user1",
				"groups":             []string{"group1"},
			},
			wantErr: ErrInvalidOAuthToken,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Create the Auth Provider.
			auth := &OIDCProvider{
				oauthConfig: &oauth2.Config{
					ClientID: clientID,
				},
				oidcVerifier:  provider.verifier,
				allowedUsers:  tc.allowedUsers,
				allowedGroups: tc.allowedGroups,
			}
			// Sign the claims to create a token.
			signedToken := provider.SignClaims(t, tc.claims)
			// Craft a test HTTP request that includes the token as a cookie.
			req := httptest.NewRequest(http.MethodGet, "/", nil)
			req.AddCookie(&http.Cookie{
				Name:  auth.getAppScopedCookieName(CookieOauthToken),
				Value: signedToken,
			})

			// Call CheckToken and verify the result.
			err := auth.CheckToken(req)
			if tc.wantErr == nil {
				expect.NoError(t, err)
			} else {
				expect.ErrorIs(t, tc.wantErr, err)
			}
		})
	}
}

func TestLogoutHandler(t *testing.T) {
	t.Helper()

	setupMockOIDC(t)

	req := httptest.NewRequest(http.MethodGet, OIDCLogoutPath, nil)
	w := httptest.NewRecorder()

	req.AddCookie(&http.Cookie{
		Name:  CookieOauthToken,
		Value: "test-token",
	})
	req.AddCookie(&http.Cookie{
		Name:  CookieOauthSessionToken,
		Value: "test-session-token",
	})

	GetDefaultAuth().(*OIDCProvider).LogoutHandler(w, req)

	if got := w.Code; got != http.StatusFound {
		t.Errorf("LogoutHandler() status = %v, want %v", got, http.StatusFound)
	}

	if got := w.Header().Get("Location"); got == "" {
		t.Error("LogoutHandler() missing redirect location")
	}

	if len(w.Header().Values("Set-Cookie")) != 2 {
		t.Error("LogoutHandler() did not clear all cookies")
	}
}
