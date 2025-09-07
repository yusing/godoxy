package auth

import (
	"crypto/rand"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/coreos/go-oidc/v3/oidc"
	"github.com/golang-jwt/jwt/v5"
	"github.com/yusing/go-proxy/internal/common"
	"golang.org/x/oauth2"

	. "github.com/yusing/go-proxy/internal/utils/testing"
)

// setupMockOIDC configures mock OIDC provider for testing.
func setupMockOIDC(t *testing.T) {
	t.Helper()

	provider := (&oidc.ProviderConfig{}).NewProvider(t.Context())
	defaultAuth = &OIDCProvider{
		oauthConfig: &oauth2.Config{
			ClientID:     "test-client",
			ClientSecret: "test-secret",
			RedirectURL:  "http://localhost/callback",
			Endpoint: oauth2.Endpoint{
				AuthURL:  "http://mock-provider/auth",
				TokenURL: "http://mock-provider/token",
			},
			Scopes: []string{oidc.ScopeOpenID, "profile", "email"},
		},
		endSessionURL: Must(url.Parse("http://mock-provider/logout")),
		oidcProvider:  provider,
		oidcVerifier: provider.Verifier(&oidc.Config{
			ClientID: "test-client",
		}),
		allowedUsers:  []string{"test-user"},
		allowedGroups: []string{"test-group1", "test-group2"},
	}
}

// discoveryDocument returns a mock OIDC discovery document.
func discoveryDocument(t *testing.T, server *httptest.Server) map[string]any {
	t.Helper()

	discovery := map[string]any{
		"issuer":                 server.URL,
		"authorization_endpoint": server.URL + "/auth",
		"token_endpoint":         server.URL + "/token",
	}

	return discovery
}

const (
	keyID    = "test-key-id"
	clientID = "test-client-id"
)

type provider struct {
	ts       *httptest.Server
	key      *rsa.PrivateKey
	verifier *oidc.IDTokenVerifier
}

func (j *provider) SignClaims(t *testing.T, claims jwt.Claims) string {
	t.Helper()

	token := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	token.Header["kid"] = keyID
	signed, err := token.SignedString(j.key)
	ExpectNoError(t, err)
	return signed
}

func setupProvider(t *testing.T) *provider {
	t.Helper()

	// Generate an RSA key pair for the test.
	privKey, err := rsa.GenerateKey(rand.Reader, 2048)
	ExpectNoError(t, err)

	// Build the matching public JWK that will be served by the endpoint.
	jwk := buildRSAJWK(t, &privKey.PublicKey, keyID)

	// Start a test server that serves the JWKS endpoint.
	ts := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/.well-known/jwks.json":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"keys": []any{jwk},
			})
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(ts.Close)

	// Create a test OIDCProvider.
	providerCtx := oidc.ClientContext(t.Context(), ts.Client())
	keySet := oidc.NewRemoteKeySet(providerCtx, ts.URL+"/.well-known/jwks.json")

	return &provider{
		ts:  ts,
		key: privKey,
		verifier: oidc.NewVerifier(ts.URL, keySet, &oidc.Config{
			ClientID: clientID, // matches audience in the token
		}),
	}
}

// buildRSAJWK is a helper to construct a minimal JWK for the JWKS endpoint.
func buildRSAJWK(t *testing.T, pub *rsa.PublicKey, kid string) map[string]any {
	t.Helper()

	nBytes := pub.N.Bytes()
	eBytes := []byte{0x01, 0x00, 0x01} // Usually 65537

	return map[string]any{
		"kty": "RSA",
		"alg": "RS256",
		"use": "sig",
		"kid": kid,
		"n":   base64.RawURLEncoding.EncodeToString(nBytes),
		"e":   base64.RawURLEncoding.EncodeToString(eBytes),
	}
}

func cleanup() {
	defaultAuth = nil
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

			defaultAuth.(*OIDCProvider).HandleAuth(w, req)

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
					Name:  CookieOauthState,
					Value: tt.state,
				})
			}
			w := httptest.NewRecorder()

			defaultAuth.(*OIDCProvider).PostAuthCallbackHandler(w, req)

			if got := w.Code; got != tt.wantStatus {
				t.Errorf("OIDCCallbackHandler() status = %v, want %v", got, tt.wantStatus)
			}

			if tt.wantStatus == http.StatusTemporaryRedirect {
				setCookie := Must(http.ParseSetCookie(w.Header().Get("Set-Cookie")))
				ExpectEqual(t, setCookie.Name, CookieOauthToken)
				ExpectTrue(t, setCookie.Value != "")
				ExpectEqual(t, setCookie.Path, "/")
				ExpectEqual(t, setCookie.SameSite, http.SameSiteLaxMode)
				ExpectEqual(t, setCookie.HttpOnly, true)
			}
		})
	}
}

func TestInitOIDC(t *testing.T) {
	setupMockOIDC(t)
	// Create a test server that serves the discovery document
	var server *httptest.Server
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		ExpectNoError(t, json.NewEncoder(w).Encode(discoveryDocument(t, server)))
	})
	server = httptest.NewServer(mux)
	t.Cleanup(server.Close)
	t.Cleanup(cleanup)

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
			issuerURL:    server.URL,
			clientID:     "client_id",
			clientSecret: "client_secret",
			allowedUsers: []string{"user1", "user2"},
			wantErr:      false,
		},
		{
			name:          "Success - Valid configuration with groups",
			issuerURL:     server.URL,
			clientID:      "client_id",
			clientSecret:  "client_secret",
			allowedGroups: []string{"group1", "group2"},
			wantErr:       false,
		},
		{
			name:          "Success - Valid configuration with users, groups and logout URL",
			issuerURL:     server.URL,
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
			_, err := NewOIDCProvider(tt.issuerURL, tt.clientID, tt.clientSecret, tt.allowedUsers, tt.allowedGroups)
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
		claims        jwt.Claims
		wantErr       error
	}{
		{
			name:         "Success - Valid token with allowed user",
			allowedUsers: []string{"user1"},
			claims: jwt.MapClaims{
				"iss":                provider.ts.URL,
				"aud":                clientID,
				"exp":                time.Now().Add(time.Hour).Unix(),
				"preferred_username": "user1",
				"groups":             []string{"group1"},
			},
		},
		{
			name:          "Success - Valid token with allowed group",
			allowedGroups: []string{"group1"},
			claims: jwt.MapClaims{
				"iss":                provider.ts.URL,
				"aud":                clientID,
				"exp":                time.Now().Add(time.Hour).Unix(),
				"preferred_username": "user1",
				"groups":             []string{"group1"},
			},
		},
		{
			name:         "Success - Server omits groups, but user is allowed",
			allowedUsers: []string{"user1"},
			claims: jwt.MapClaims{
				"iss":                provider.ts.URL,
				"aud":                clientID,
				"exp":                time.Now().Add(time.Hour).Unix(),
				"preferred_username": "user1",
			},
		},
		{
			name:          "Success - Server omits preferred_username, but group is allowed",
			allowedGroups: []string{"group1"},
			claims: jwt.MapClaims{
				"iss":    provider.ts.URL,
				"aud":    clientID,
				"exp":    time.Now().Add(time.Hour).Unix(),
				"groups": []string{"group1"},
			},
		},
		{
			name:          "Success - Valid token with allowed user and group",
			allowedUsers:  []string{"user1"},
			allowedGroups: []string{"group1"},
			claims: jwt.MapClaims{
				"iss":                provider.ts.URL,
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
			claims: jwt.MapClaims{
				"iss":                provider.ts.URL,
				"aud":                clientID,
				"exp":                time.Now().Add(time.Hour).Unix(),
				"preferred_username": "user1",
				"groups":             []string{"group1"},
			},
			wantErr: ErrUserNotAllowed,
		},
		{
			name: "Error - Server returns incorrect issuer",
			claims: jwt.MapClaims{
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
			claims: jwt.MapClaims{
				"iss":                provider.ts.URL,
				"aud":                "some-other-audience",
				"exp":                time.Now().Add(time.Hour).Unix(),
				"preferred_username": "user1",
				"groups":             []string{"group1"},
			},
			wantErr: ErrInvalidOAuthToken,
		},
		{
			name: "Error - Server returns expired token",
			claims: jwt.MapClaims{
				"iss":                provider.ts.URL,
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
				ExpectNoError(t, err)
			} else {
				ExpectError(t, tc.wantErr, err)
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

	defaultAuth.(*OIDCProvider).LogoutHandler(w, req)

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
