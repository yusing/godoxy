package e2e

import (
	"crypto/rand"
	"crypto/rsa"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync"
	"testing"
	"time"

	"github.com/coreos/go-oidc/v3/oidc"
	"github.com/coreos/go-oidc/v3/oidc/oidctest"
	"github.com/stretchr/testify/require"
)

type fakeOIDCRefreshMode uint8

const (
	fakeOIDCRefreshAllowed fakeOIDCRefreshMode = iota
	fakeOIDCRefreshMissingClaims
	fakeOIDCRefreshDisallowed
	fakeOIDCRefreshRejected
)

type fakeOIDCIdP struct {
	server     *httptest.Server
	discovery  *oidctest.Server
	privateKey *rsa.PrivateKey
	clientID   string

	mu           sync.Mutex
	callbackURL  string
	refreshMode  fakeOIDCRefreshMode
	refreshCalls int
}

func newFakeOIDCIdP(t *testing.T, clientID string) *fakeOIDCIdP {
	t.Helper()

	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)

	discovery := &oidctest.Server{
		PublicKeys: []oidctest.PublicKey{
			{
				PublicKey: privateKey.Public(),
				KeyID:     "fake-idp-key",
				Algorithm: oidc.RS256,
			},
		},
	}
	idp := &fakeOIDCIdP{
		discovery:  discovery,
		privateKey: privateKey,
		clientID:   clientID,
	}
	idp.server = httptest.NewServer(http.HandlerFunc(idp.serveHTTP))
	discovery.SetIssuer(idp.server.URL)
	t.Cleanup(idp.server.Close)
	return idp
}

func (idp *fakeOIDCIdP) setCallbackURL(callbackURL string) {
	idp.mu.Lock()
	idp.callbackURL = callbackURL
	idp.mu.Unlock()
}

func (idp *fakeOIDCIdP) setRefreshMode(mode fakeOIDCRefreshMode) int {
	idp.mu.Lock()
	idp.refreshMode = mode
	calls := idp.refreshCalls
	idp.mu.Unlock()
	return calls
}

func (idp *fakeOIDCIdP) refreshCallCount() int {
	idp.mu.Lock()
	defer idp.mu.Unlock()
	return idp.refreshCalls
}

func (idp *fakeOIDCIdP) serveHTTP(w http.ResponseWriter, r *http.Request) {
	switch r.URL.Path {
	case "/.well-known/openid-configuration", "/keys":
		idp.discovery.ServeHTTP(w, r)
	case "/auth":
		idp.serveAuthorization(w, r)
	case "/token":
		idp.serveToken(w, r)
	default:
		http.NotFound(w, r)
	}
}

func (idp *fakeOIDCIdP) serveAuthorization(w http.ResponseWriter, r *http.Request) {
	idp.mu.Lock()
	callbackURL := idp.callbackURL
	idp.mu.Unlock()
	if callbackURL == "" {
		http.Error(w, "fake IdP callback URL is not configured", http.StatusInternalServerError)
		return
	}

	callback, err := url.Parse(callbackURL)
	if err != nil {
		http.Error(w, "fake IdP callback URL is invalid", http.StatusInternalServerError)
		return
	}
	query := callback.Query()
	query.Set("code", "initial-code")
	query.Set("state", r.URL.Query().Get("state"))
	callback.RawQuery = query.Encode()
	http.Redirect(w, r, callback.String(), http.StatusFound)
}

func (idp *fakeOIDCIdP) serveToken(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "invalid token request", http.StatusBadRequest)
		return
	}

	switch r.Form.Get("grant_type") {
	case "authorization_code":
		idp.writeToken(w, idp.signIDToken("test-user", []string{"test-group"}, time.Now().Add(time.Hour)), "initial-refresh-token")
	case "refresh_token":
		idp.mu.Lock()
		idp.refreshCalls++
		mode := idp.refreshMode
		idp.mu.Unlock()

		switch mode {
		case fakeOIDCRefreshAllowed:
			idp.writeToken(w, idp.signIDToken("test-user", []string{"test-group"}, time.Now().Add(time.Hour)), "replacement-refresh-token")
		case fakeOIDCRefreshMissingClaims:
			idp.writeToken(w, idp.signIDToken("", nil, time.Now().Add(time.Hour)), "replacement-refresh-token")
		case fakeOIDCRefreshDisallowed:
			idp.writeToken(w, idp.signIDToken("blocked-user", nil, time.Now().Add(time.Hour)), "replacement-refresh-token")
		case fakeOIDCRefreshRejected:
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte(`{"error":"invalid_grant"}`))
		default:
			http.Error(w, "unknown fake refresh mode", http.StatusInternalServerError)
		}
	default:
		http.Error(w, "unsupported grant type", http.StatusBadRequest)
	}
}

func (idp *fakeOIDCIdP) expiredIDToken() string {
	return idp.signIDToken("test-user", []string{"test-group"}, time.Now().Add(-time.Hour))
}

func (idp *fakeOIDCIdP) signIDToken(username string, groups []string, expiry time.Time) string {
	claims := map[string]any{
		"iss": idp.server.URL,
		"aud": idp.clientID,
		"exp": expiry.Unix(),
		"sub": "fake-subject",
	}
	if username != "" {
		claims["preferred_username"] = username
	}
	if groups != nil {
		claims["groups"] = groups
	}
	rawClaims, err := json.Marshal(claims)
	if err != nil {
		panic(err)
	}
	return oidctest.SignIDToken(idp.privateKey, "fake-idp-key", oidc.RS256, string(rawClaims))
}

func (idp *fakeOIDCIdP) writeToken(w http.ResponseWriter, idToken, refreshToken string) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"access_token":  "fake-access-token",
		"token_type":    "Bearer",
		"refresh_token": refreshToken,
		"expires_in":    3600,
		"id_token":      idToken,
	})
}
