package e2e

import (
	"context"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/yusing/godoxy/internal/auth"
	"github.com/yusing/godoxy/internal/common"
	"github.com/yusing/godoxy/internal/route"
	"github.com/yusing/godoxy/internal/route/rules"
	rulepresets "github.com/yusing/godoxy/internal/route/rules/presets"
	"github.com/yusing/godoxy/internal/routeimpl"
	"github.com/yusing/godoxy/webui"
)

const fakeOIDCClientID = "webui-e2e-client"

func TestEmbeddedWebUIOIDCRefreshE2E(t *testing.T) {
	idp := newFakeOIDCIdP(t, fakeOIDCClientID)
	appURL := setupOIDCWebUI(t, idp)

	t.Run("expired document session refreshes and reaches the SPA", func(t *testing.T) {
		client := authenticatedOIDCClient(t, appURL, idp)
		replaceAppCookie(t, client, appURL, auth.CookieOauthToken, idp.expiredIDToken())
		before := idp.setRefreshMode(fakeOIDCRefreshAllowed)

		refreshResponse := doOIDCRequest(t, client, http.MethodGet, appURL+"/routes?tab=servers&tab=health", "text/html", nil)
		closeOIDCResponse(t, refreshResponse)

		require.Equal(t, http.StatusFound, refreshResponse.StatusCode)
		require.Equal(t, "/routes?tab=servers&tab=health", refreshResponse.Header.Get("Location"))
		require.Equal(t, before+1, idp.refreshCallCount())
		assertSetCookiePrefix(t, refreshResponse, auth.CookieOauthToken)
		assertSetCookiePrefix(t, refreshResponse, auth.CookieOauthSessionToken)

		documentResponse := doOIDCRequest(t, client, http.MethodGet, resolveOIDCLocation(t, refreshResponse), "text/html", nil)
		documentBody := closeOIDCResponse(t, documentResponse)

		require.Equal(t, http.StatusOK, documentResponse.StatusCode)
		require.Contains(t, documentBody, "<!DOCTYPE html>")
		require.NotEmpty(t, strings.TrimSpace(documentBody))
		require.Equal(t, before+1, idp.refreshCallCount(), "the redirected document request must not refresh again")
	})

	for _, scenario := range []struct {
		name string
		mode fakeOIDCRefreshMode
	}{
		{name: "IdP rejects refresh", mode: fakeOIDCRefreshRejected},
		{name: "refreshed token is missing identity claims", mode: fakeOIDCRefreshMissingClaims},
		{name: "refreshed identity is no longer allowed", mode: fakeOIDCRefreshDisallowed},
	} {
		t.Run(scenario.name+" without a redirect loop", func(t *testing.T) {
			client := authenticatedOIDCClient(t, appURL, idp)
			replaceAppCookie(t, client, appURL, auth.CookieOauthToken, idp.expiredIDToken())
			before := idp.setRefreshMode(scenario.mode)

			failureResponse := doOIDCRequest(t, client, http.MethodGet, appURL+"/routes", "text/html", nil)
			closeOIDCResponse(t, failureResponse)

			require.Equal(t, http.StatusFound, failureResponse.StatusCode)
			require.Equal(t, "/", failureResponse.Header.Get("Location"))
			require.Greater(t, idp.refreshCallCount(), before)
			assertClearedCookiePrefix(t, failureResponse, auth.CookieOauthToken)
			assertClearedCookiePrefix(t, failureResponse, auth.CookieOauthSessionToken)

			loginResponse := doOIDCRequest(t, client, http.MethodGet, resolveOIDCLocation(t, failureResponse), "text/html", nil)
			closeOIDCResponse(t, loginResponse)

			require.Equal(t, http.StatusFound, loginResponse.StatusCode)
			require.True(t, strings.HasPrefix(loginResponse.Header.Get("Location"), idp.server.URL+"/auth?"))
			require.NotEqual(t, "/routes", loginResponse.Header.Get("Location"))
		})
	}

	t.Run("expired token without a refresh session starts the IdP flow", func(t *testing.T) {
		client := authenticatedOIDCClient(t, appURL, idp)
		replaceAppCookie(t, client, appURL, auth.CookieOauthToken, idp.expiredIDToken())
		removeAppCookie(t, client, appURL, auth.CookieOauthSessionToken)
		before := idp.setRefreshMode(fakeOIDCRefreshAllowed)

		response := doOIDCRequest(t, client, http.MethodGet, appURL+"/routes", "text/html", nil)
		closeOIDCResponse(t, response)

		require.Equal(t, http.StatusFound, response.StatusCode)
		require.True(t, strings.HasPrefix(response.Header.Get("Location"), idp.server.URL+"/auth?"))
		require.Equal(t, before, idp.refreshCallCount())
	})

	t.Run("API and WebSocket requests remain API responses", func(t *testing.T) {
		for _, request := range []struct {
			name    string
			headers map[string]string
		}{
			{name: "JSON API", headers: map[string]string{"Accept": "application/json"}},
			{name: "WebSocket", headers: map[string]string{"Connection": "Upgrade", "Upgrade": "websocket"}},
		} {
			t.Run(request.name, func(t *testing.T) {
				client := authenticatedOIDCClient(t, appURL, idp)
				replaceAppCookie(t, client, appURL, auth.CookieOauthToken, idp.expiredIDToken())
				before := idp.setRefreshMode(fakeOIDCRefreshAllowed)

				response := doOIDCRequest(t, client, http.MethodGet, appURL+"/api/v1/health", "", request.headers)
				closeOIDCResponse(t, response)

				require.Equal(t, http.StatusUnauthorized, response.StatusCode)
				require.Empty(t, response.Header.Get("Location"))
				require.Equal(t, before, idp.refreshCallCount())
			})
		}
	})

	t.Run("unsafe document request is refreshed but never replayed", func(t *testing.T) {
		client := authenticatedOIDCClient(t, appURL, idp)
		replaceAppCookie(t, client, appURL, auth.CookieOauthToken, idp.expiredIDToken())
		before := idp.setRefreshMode(fakeOIDCRefreshAllowed)

		response := doOIDCRequest(t, client, http.MethodPost, appURL+"/routes", "text/html", nil)
		closeOIDCResponse(t, response)

		require.Equal(t, http.StatusForbidden, response.StatusCode)
		require.Empty(t, response.Header.Get("Location"))
		require.Equal(t, before+1, idp.refreshCallCount())

		documentResponse := doOIDCRequest(t, client, http.MethodGet, appURL+"/routes", "text/html", nil)
		documentBody := closeOIDCResponse(t, documentResponse)
		require.Equal(t, http.StatusOK, documentResponse.StatusCode)
		require.Contains(t, documentBody, "<!DOCTYPE html>")
	})

	t.Run("non-HTML document request is rejected without refresh", func(t *testing.T) {
		client := authenticatedOIDCClient(t, appURL, idp)
		replaceAppCookie(t, client, appURL, auth.CookieOauthToken, idp.expiredIDToken())
		before := idp.setRefreshMode(fakeOIDCRefreshAllowed)

		response := doOIDCRequest(t, client, http.MethodGet, appURL+"/routes", "application/json", nil)
		closeOIDCResponse(t, response)

		require.Equal(t, http.StatusForbidden, response.StatusCode)
		require.Empty(t, response.Header.Get("Location"))
		require.Equal(t, before, idp.refreshCallCount())
	})

	t.Run("auth check refreshes without browser navigation", func(t *testing.T) {
		client := authenticatedOIDCClient(t, appURL, idp)
		replaceAppCookie(t, client, appURL, auth.CookieOauthToken, idp.expiredIDToken())
		before := idp.setRefreshMode(fakeOIDCRefreshAllowed)

		response := doOIDCRequest(t, client, http.MethodHead, appURL+"/api/v1/auth/check", "text/html", nil)
		closeOIDCResponse(t, response)

		require.Equal(t, http.StatusOK, response.StatusCode)
		require.Empty(t, response.Header.Get("Location"))
		require.Equal(t, before+1, idp.refreshCallCount())
		assertSetCookiePrefix(t, response, auth.CookieOauthToken)
		assertSetCookiePrefix(t, response, auth.CookieOauthSessionToken)
	})
}

func setupOIDCWebUI(t *testing.T, idp *fakeOIDCIdP) string {
	t.Helper()

	previous := struct {
		isTest              bool
		apiJWTSecure        bool
		apiJWTSecret        []byte
		apiJWTTokenTTL      time.Duration
		debugDisableAuth    bool
		oidcIssuerURL       string
		oidcClientID        string
		oidcClientSecret    string
		oidcScopes          []string
		oidcAllowedUsers    []string
		oidcAllowedGroups   []string
		oidcRateLimit       int
		oidcRateLimitPeriod time.Duration
	}{
		isTest:              common.IsTest,
		apiJWTSecure:        common.APIJWTSecure,
		apiJWTSecret:        common.APIJWTSecret,
		apiJWTTokenTTL:      common.APIJWTTokenTTL,
		debugDisableAuth:    common.DebugDisableAuth,
		oidcIssuerURL:       common.OIDCIssuerURL,
		oidcClientID:        common.OIDCClientID,
		oidcClientSecret:    common.OIDCClientSecret,
		oidcScopes:          common.OIDCScopes,
		oidcAllowedUsers:    common.OIDCAllowedUsers,
		oidcAllowedGroups:   common.OIDCAllowedGroups,
		oidcRateLimit:       common.OIDCRateLimit,
		oidcRateLimitPeriod: common.OIDCRateLimitPeriod,
	}
	previousAuthHandler := rules.GetAuthHandler()
	previousAPIHandler, hadAPIHandler := rules.GetHandler("api")
	previousProviderPresent := auth.GetDefaultAuth() != nil

	common.IsTest = false
	common.APIJWTSecure = false
	common.APIJWTSecret = []byte("webui-oidc-e2e-session-secret")
	common.APIJWTTokenTTL = time.Hour
	common.DebugDisableAuth = false
	common.OIDCIssuerURL = idp.server.URL
	common.OIDCClientID = fakeOIDCClientID
	common.OIDCClientSecret = "fake-client-secret"
	common.OIDCScopes = []string{"openid", "profile", "groups"}
	common.OIDCAllowedUsers = []string{"test-user"}
	common.OIDCAllowedGroups = nil
	common.OIDCRateLimit = 1_000
	common.OIDCRateLimitPeriod = time.Nanosecond
	require.NoError(t, auth.Initialize(t.Context()))

	rules.InitAuthHandler(auth.AuthOrProceed)
	rules.ReplaceHandler("api", oidcE2EAPIHandler())

	webuiRules, ok := rulepresets.GetRulePreset("webui.yml")
	require.True(t, ok)
	fileServer, err := routeimpl.NewFileServer(&route.Route{
		Root:   "embed://webui",
		RootFS: webui.Dist(),
		SPA:    true,
		Index:  "_shell.html",
		Rules:  webuiRules,
	})
	require.NoError(t, err)
	appServer := httptest.NewServer(webuiRules.BuildHandler(fileServer.ServeHTTP))
	idp.setCallbackURL(appServer.URL + auth.OIDCPostAuthPath)

	t.Cleanup(func() {
		appServer.Close()
		rules.InitAuthHandler(previousAuthHandler)
		if hadAPIHandler {
			rules.ReplaceHandler("api", previousAPIHandler)
		} else {
			rules.ReplaceHandler("api", nil)
		}

		common.IsTest = previous.isTest
		common.APIJWTSecure = previous.apiJWTSecure
		common.APIJWTSecret = previous.apiJWTSecret
		common.APIJWTTokenTTL = previous.apiJWTTokenTTL
		common.DebugDisableAuth = previous.debugDisableAuth
		common.OIDCIssuerURL = previous.oidcIssuerURL
		common.OIDCClientID = previous.oidcClientID
		common.OIDCClientSecret = previous.oidcClientSecret
		common.OIDCScopes = previous.oidcScopes
		common.OIDCAllowedUsers = previous.oidcAllowedUsers
		common.OIDCAllowedGroups = previous.oidcAllowedGroups
		common.OIDCRateLimit = previous.oidcRateLimit
		common.OIDCRateLimitPeriod = previous.oidcRateLimitPeriod

		if previousProviderPresent {
			if err := auth.Initialize(context.Background()); err != nil {
				t.Errorf("restore authentication provider: %v", err)
			}
			return
		}
		restoredDisableAuth := common.DebugDisableAuth
		common.DebugDisableAuth = true
		if err := auth.Initialize(context.Background()); err != nil {
			t.Errorf("clear authentication provider: %v", err)
		}
		common.DebugDisableAuth = restoredDisableAuth
	})
	return appServer.URL
}

func oidcE2EAPIHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/auth/callback":
			auth.GetDefaultAuth().PostAuthCallbackHandler(w, r)
		case "/api/v1/auth/check":
			auth.AuthCheckHandler(w, r)
		default:
			if err := auth.GetDefaultAuth().CheckToken(r); err != nil {
				http.Error(w, "unauthorized", http.StatusUnauthorized)
				return
			}
			w.WriteHeader(http.StatusOK)
		}
	})
}

func authenticatedOIDCClient(t *testing.T, appURL string, idp *fakeOIDCIdP) *http.Client {
	t.Helper()
	jar, err := cookiejar.New(nil)
	require.NoError(t, err)
	client := &http.Client{
		Jar: jar,
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}

	loginResponse := doOIDCRequest(t, client, http.MethodGet, appURL+"/routes", "text/html", nil)
	closeOIDCResponse(t, loginResponse)
	require.Equal(t, http.StatusFound, loginResponse.StatusCode)
	require.True(t, strings.HasPrefix(loginResponse.Header.Get("Location"), idp.server.URL+"/auth?"))

	providerResponse := doOIDCRequest(t, client, http.MethodGet, resolveOIDCLocation(t, loginResponse), "text/html", nil)
	closeOIDCResponse(t, providerResponse)
	require.Equal(t, http.StatusFound, providerResponse.StatusCode)
	require.True(t, strings.HasPrefix(providerResponse.Header.Get("Location"), appURL+auth.OIDCPostAuthPath+"?"))

	callbackResponse := doOIDCRequest(t, client, http.MethodGet, resolveOIDCLocation(t, providerResponse), "text/html", nil)
	closeOIDCResponse(t, callbackResponse)
	require.Equal(t, http.StatusFound, callbackResponse.StatusCode)
	require.Equal(t, "/", callbackResponse.Header.Get("Location"))
	assertSetCookiePrefix(t, callbackResponse, auth.CookieOauthToken)
	assertSetCookiePrefix(t, callbackResponse, auth.CookieOauthSessionToken)

	homeResponse := doOIDCRequest(t, client, http.MethodGet, resolveOIDCLocation(t, callbackResponse), "text/html", nil)
	homeBody := closeOIDCResponse(t, homeResponse)
	require.Equal(t, http.StatusOK, homeResponse.StatusCode)
	require.Contains(t, homeBody, "<!DOCTYPE html>")

	initialSessionCookies := appCookiesWithPrefix(t, client, appURL, auth.CookieOauthSessionToken)
	t.Cleanup(func() {
		provider := auth.GetDefaultAuth()
		if provider == nil {
			return
		}
		cookies := append(initialSessionCookies, appCookiesWithPrefix(t, client, appURL, auth.CookieOauthSessionToken)...)
		seen := make(map[string]struct{}, len(cookies))
		for _, cookie := range cookies {
			if _, ok := seen[cookie.Value]; ok {
				continue
			}
			seen[cookie.Value] = struct{}{}
			req := httptest.NewRequest(http.MethodGet, appURL+auth.OIDCLogoutPath, nil)
			req.AddCookie(cookie)
			provider.LogoutHandler(httptest.NewRecorder(), req)
		}
	})
	return client
}

func doOIDCRequest(t *testing.T, client *http.Client, method, requestURL, accept string, headers map[string]string) *http.Response {
	t.Helper()
	req, err := http.NewRequestWithContext(t.Context(), method, requestURL, nil)
	require.NoError(t, err)
	if accept != "" {
		req.Header.Set("Accept", accept)
	}
	for name, value := range headers {
		req.Header.Set(name, value)
	}
	response, err := client.Do(req)
	require.NoError(t, err)
	return response
}

func closeOIDCResponse(t *testing.T, response *http.Response) string {
	t.Helper()
	body, err := io.ReadAll(response.Body)
	require.NoError(t, err)
	require.NoError(t, response.Body.Close())
	return string(body)
}

func resolveOIDCLocation(t *testing.T, response *http.Response) string {
	t.Helper()
	location, err := response.Location()
	require.NoError(t, err)
	return response.Request.URL.ResolveReference(location).String()
}

func replaceAppCookie(t *testing.T, client *http.Client, appURL, prefix, value string) {
	t.Helper()
	parsedURL, err := url.Parse(appURL)
	require.NoError(t, err)
	cookies := appCookiesWithPrefix(t, client, appURL, prefix)
	require.Len(t, cookies, 1)
	client.Jar.SetCookies(parsedURL, []*http.Cookie{{Name: cookies[0].Name, Value: value, Path: "/"}})
}

func removeAppCookie(t *testing.T, client *http.Client, appURL, prefix string) {
	t.Helper()
	parsedURL, err := url.Parse(appURL)
	require.NoError(t, err)
	cookies := appCookiesWithPrefix(t, client, appURL, prefix)
	require.Len(t, cookies, 1)
	client.Jar.SetCookies(parsedURL, []*http.Cookie{{Name: cookies[0].Name, Path: "/", MaxAge: -1}})
}

func appCookiesWithPrefix(t *testing.T, client *http.Client, appURL, prefix string) []*http.Cookie {
	t.Helper()
	parsedURL, err := url.Parse(appURL)
	require.NoError(t, err)
	var matches []*http.Cookie
	for _, cookie := range client.Jar.Cookies(parsedURL) {
		if strings.HasPrefix(cookie.Name, prefix+"_") || cookie.Name == prefix {
			matches = append(matches, cookie)
		}
	}
	return matches
}

func assertSetCookiePrefix(t *testing.T, response *http.Response, prefix string) {
	t.Helper()
	for _, cookie := range response.Cookies() {
		if (strings.HasPrefix(cookie.Name, prefix+"_") || cookie.Name == prefix) && cookie.MaxAge >= 0 && cookie.Value != "" {
			return
		}
	}
	t.Errorf("response did not set cookie with prefix %q", prefix)
}

func assertClearedCookiePrefix(t *testing.T, response *http.Response, prefix string) {
	t.Helper()
	for _, cookie := range response.Cookies() {
		if (strings.HasPrefix(cookie.Name, prefix+"_") || cookie.Name == prefix) && cookie.MaxAge < 0 {
			return
		}
	}
	t.Errorf("response did not clear cookie with prefix %q", prefix)
}
