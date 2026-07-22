package main

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/coreos/go-oidc/v3/oidc/oidctest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	e2eChildEnv = "GODOXY_E2E_CHILD"
	e2eUser     = "e2e-user"
	e2ePassword = "e2e-password"
	e2eRuleHost = "godoxy"
)

// TestGodoxyE2EProcess is the subprocess entry point used by
// TestAuthCallbackStartupMatrix. Environment-backed globals are initialized
// before this function runs, so each subprocess gets an isolated environment.
func TestGodoxyE2EProcess(t *testing.T) {
	if os.Getenv(e2eChildEnv) != "1" {
		t.Skip("subprocess helper")
	}
	main()
}

func TestAuthCallbackStartupMatrix(t *testing.T) {
	oidcProvider := &oidctest.Server{}
	oidcIssuer := httptest.NewServer(oidcProvider)
	oidcProvider.SetIssuer(oidcIssuer.URL)
	t.Cleanup(oidcIssuer.Close)

	tests := []struct {
		name      string
		oidc      bool
		envJWTKey bool
	}{
		{name: "basic/no_env_jwt"},
		{name: "basic/env_jwt", envJWTKey: true},
		{name: "oidc/no_env_jwt", oidc: true},
		{name: "oidc/env_jwt", oidc: true, envJWTKey: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			addresses := unusedLoopbackAddresses(t, 2)
			apiAddr, proxyAddr := addresses[0], addresses[1]
			oidcURL := optionalOIDCURL(tt.oidc, oidcIssuer.URL)
			process := startGodoxyE2EProcess(t, godoxyE2EProcessConfig{
				apiAddr:   apiAddr,
				proxyAddr: proxyAddr,
				oidcURL:   oidcURL,
				envJWTKey: tt.envJWTKey,
			})

			directBaseURL := "http://" + apiAddr
			rulesBaseURL := "http://" + proxyAddr
			process.waitReady(t, directBaseURL, rulesBaseURL)
			if tt.envJWTKey {
				assert.NotContains(t, process.logs(), "API_JWT_SECRET is not set, using random key")
			} else {
				assert.Contains(t, process.logs(), "API_JWT_SECRET is not set, using random key")
			}

			t.Run("direct_api_callback", func(t *testing.T) {
				exerciseAuthCallback(t, authCallbackScope{
					baseURL: directBaseURL,
					host:    apiAddr,
					oidcURL: oidcURL,
				})
			})

			t.Run("rules_trigger", func(t *testing.T) {
				exerciseAuthCallback(t, authCallbackScope{
					baseURL: rulesBaseURL,
					host:    e2eRuleHost,
					oidcURL: oidcURL,
				})
			})
		})
	}
}

func TestStartupRejectsMissingBasicAuthCredentials(t *testing.T) {
	address := unusedLoopbackAddresses(t, 1)[0]
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestGodoxyE2EProcess$")
	cmd.Dir = t.TempDir()
	cmd.Env = godoxyE2EEnvironment(godoxyE2EProcessConfig{
		apiAddr:         address,
		proxyAddr:       "127.0.0.1:0",
		omitCredentials: true,
	})
	output, err := cmd.CombinedOutput()

	require.Error(t, err, "GoDoxy started without basic-auth credentials")
	require.NoError(t, ctx.Err(), "timed out waiting for GoDoxy to reject its configuration")
	assert.Contains(t, string(output), "GODOXY_API_USER and GODOXY_API_PASSWORD must be set")
}

type authCallbackScope struct {
	baseURL string
	host    string
	oidcURL string
}

func exerciseAuthCallback(t *testing.T, scope authCallbackScope) {
	t.Helper()
	jar, err := cookiejar.New(nil)
	require.NoError(t, err)
	client := &http.Client{
		Jar:     jar,
		Timeout: 3 * time.Second,
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}

	if scope.oidcURL != "" {
		exerciseOIDCCallback(t, client, scope)
		return
	}
	exerciseBasicCallback(t, client, scope)
}

func exerciseBasicCallback(t *testing.T, client *http.Client, scope authCallbackScope) {
	t.Helper()
	body, err := json.Marshal(map[string]string{
		"username": e2eUser,
		"password": e2ePassword,
	})
	require.NoError(t, err)

	req, err := http.NewRequest(
		http.MethodPost,
		scope.baseURL+"/api/v1/auth/callback",
		strings.NewReader(string(body)),
	)
	require.NoError(t, err)
	req.Host = scope.host
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Origin", "http://"+scope.host)

	resp, err := client.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	assert.Equal(t, http.StatusOK, resp.StatusCode, responseBody(t, resp))
	assert.True(t, responseHasCookie(resp, "godoxy_token"), "missing basic-auth session cookie")
}

func exerciseOIDCCallback(t *testing.T, client *http.Client, scope authCallbackScope) {
	t.Helper()
	loginReq, err := http.NewRequest(
		http.MethodPost,
		scope.baseURL+"/api/v1/auth/login",
		http.NoBody,
	)
	require.NoError(t, err)
	loginReq.Host = scope.host
	loginReq.Header.Set("Accept", "text/html")
	loginReq.Header.Set("Origin", "http://"+scope.host)

	loginResp, err := client.Do(loginReq)
	require.NoError(t, err)
	loginBody := responseBody(t, loginResp)
	require.NoError(t, loginResp.Body.Close())
	require.Equal(t, http.StatusFound, loginResp.StatusCode, loginBody)

	location, err := url.Parse(loginResp.Header.Get("Location"))
	require.NoError(t, err)
	locationWithoutQuery := *location
	locationWithoutQuery.RawQuery = ""
	assert.Equal(t, scope.oidcURL+"/auth", locationWithoutQuery.String())
	state := location.Query().Get("state")
	require.NotEmpty(t, state, "OIDC login redirect has no state")

	callbackURL := scope.baseURL + "/api/v1/auth/callback?code=e2e-code&state=" + url.QueryEscape(state)
	callbackReq, err := http.NewRequest(http.MethodGet, callbackURL, http.NoBody)
	require.NoError(t, err)
	callbackReq.Host = scope.host

	callbackResp, err := client.Do(callbackReq)
	require.NoError(t, err)
	defer callbackResp.Body.Close()
	assert.Equal(t, http.StatusFound, callbackResp.StatusCode, responseBody(t, callbackResp))
	assert.Equal(t, "/", callbackResp.Header.Get("Location"))
	assert.True(t, responseHasCookiePrefix(callbackResp, "godoxy_oauth_token_"), "missing OIDC token cookie")
}

func responseHasCookie(resp *http.Response, name string) bool {
	for _, cookie := range resp.Cookies() {
		if cookie.Name == name {
			return true
		}
	}
	return false
}

func responseHasCookiePrefix(resp *http.Response, prefix string) bool {
	for _, cookie := range resp.Cookies() {
		if strings.HasPrefix(cookie.Name, prefix) {
			return true
		}
	}
	return false
}

func responseBody(t *testing.T, resp *http.Response) string {
	t.Helper()
	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	return string(body)
}

type godoxyE2EProcessConfig struct {
	apiAddr         string
	proxyAddr       string
	oidcURL         string
	envJWTKey       bool
	omitCredentials bool
}

type godoxyE2EProcess struct {
	cmd     *exec.Cmd
	done    <-chan error
	logFile *os.File
}

func startGodoxyE2EProcess(t *testing.T, cfg godoxyE2EProcessConfig) *godoxyE2EProcess {
	t.Helper()
	workDir := t.TempDir()
	logFile, err := os.Create(filepath.Join(workDir, "godoxy.log"))
	require.NoError(t, err)

	cmd := exec.Command(os.Args[0], "-test.run=^TestGodoxyE2EProcess$")
	cmd.Dir = workDir
	cmd.Env = godoxyE2EEnvironment(cfg)
	cmd.Stdout = logFile
	cmd.Stderr = logFile
	require.NoError(t, cmd.Start())

	done := make(chan error, 1)
	go func() {
		done <- cmd.Wait()
	}()

	process := &godoxyE2EProcess{cmd: cmd, done: done, logFile: logFile}
	t.Cleanup(func() {
		process.stop(t)
	})
	return process
}

func godoxyE2EEnvironment(cfg godoxyE2EProcessConfig) []string {
	keys := []string{
		"E2E_CHILD",
		"API_ADDR",
		"HTTP_ADDR",
		"HTTPS_ADDR",
		"LOCAL_API_ADDR",
		"API_USER",
		"API_PASSWORD",
		"API_JWT_SECRET",
		"API_JWT_SECURE",
		"API_JWT_TOKEN_TTL",
		"API_SKIP_ORIGIN_CHECK",
		"DEBUG_DISABLE_AUTH",
		"OIDC_ISSUER_URL",
		"OIDC_CLIENT_ID",
		"OIDC_CLIENT_SECRET",
		"OIDC_ALLOWED_USERS",
		"OIDC_ALLOWED_GROUPS",
		"OIDC_SCOPES",
		"OIDC_RATE_LIMIT",
		"OIDC_RATE_LIMIT_PERIOD",
		"FRONTEND_ALIASES",
		"INIT_TIMEOUT",
		"DEBUG",
		"TRACE",
	}
	env := withoutEnvironmentKeys(os.Environ(), keys)
	env = append(env,
		e2eChildEnv+"=1",
		"GODOXY_API_ADDR="+cfg.apiAddr,
		"GODOXY_HTTP_ADDR="+cfg.proxyAddr,
		"GODOXY_HTTPS_ADDR=127.0.0.1:0",
		"GODOXY_API_JWT_SECURE=false",
		"GODOXY_DEBUG_DISABLE_AUTH=false",
		"GODOXY_FRONTEND_ALIASES="+e2eRuleHost,
		"GODOXY_INIT_TIMEOUT=20s",
		"GODOXY_DEBUG=false",
		"GODOXY_TRACE=false",
	)
	if !cfg.omitCredentials {
		env = append(env,
			"GODOXY_API_USER="+e2eUser,
			"GODOXY_API_PASSWORD="+e2ePassword,
		)
	}
	if cfg.envJWTKey {
		key := base64.StdEncoding.EncodeToString([]byte("0123456789abcdef0123456789abcdef"))
		env = append(env, "GODOXY_API_JWT_SECRET="+key)
	}
	if cfg.oidcURL != "" {
		env = append(env,
			"GODOXY_OIDC_ISSUER_URL="+cfg.oidcURL,
			"GODOXY_OIDC_CLIENT_ID=e2e-client",
			"GODOXY_OIDC_CLIENT_SECRET=e2e-secret",
			"GODOXY_OIDC_ALLOWED_USERS="+e2eUser,
		)
	}
	return env
}

func withoutEnvironmentKeys(environ, keys []string) []string {
	blocked := make(map[string]struct{}, len(keys)*3)
	for _, key := range keys {
		blocked[key] = struct{}{}
		blocked["GODOXY_"+key] = struct{}{}
		blocked["GOPROXY_"+key] = struct{}{}
	}

	filtered := make([]string, 0, len(environ))
	for _, entry := range environ {
		key, _, _ := strings.Cut(entry, "=")
		if _, found := blocked[key]; !found {
			filtered = append(filtered, entry)
		}
	}
	return filtered
}

func optionalOIDCURL(enabled bool, issuerURL string) string {
	if enabled {
		return issuerURL
	}
	return ""
}

func unusedLoopbackAddresses(t *testing.T, count int) []string {
	t.Helper()
	listeners := make([]net.Listener, 0, count)
	addresses := make([]string, 0, count)
	for range count {
		listener, err := net.Listen("tcp", "127.0.0.1:0")
		require.NoError(t, err)
		listeners = append(listeners, listener)
		addresses = append(addresses, listener.Addr().String())
	}
	for _, listener := range listeners {
		require.NoError(t, listener.Close())
	}
	return addresses
}

func (process *godoxyE2EProcess) waitReady(t *testing.T, directBaseURL, rulesBaseURL string) {
	t.Helper()
	client := &http.Client{Timeout: 500 * time.Millisecond}
	deadline := time.Now().Add(15 * time.Second)
	var lastError error

	for time.Now().Before(deadline) {
		directReady := endpointReady(client, directBaseURL+"/api/v1/version", "")
		rulesReady := endpointReady(client, rulesBaseURL+"/api/v1/version", e2eRuleHost)
		if directReady == nil && rulesReady == nil {
			return
		}
		lastError = fmt.Errorf("direct API: %v; rules: %v", directReady, rulesReady)
		time.Sleep(50 * time.Millisecond)
	}

	t.Fatalf("GoDoxy did not become ready: %v\n%s", lastError, process.logs())
}

func endpointReady(client *http.Client, target, host string) error {
	req, err := http.NewRequest(http.MethodGet, target, http.NoBody)
	if err != nil {
		return err
	}
	if host != "" {
		req.Host = host
	}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("status %s", resp.Status)
	}
	return nil
}

func (process *godoxyE2EProcess) logs() string {
	_ = process.logFile.Sync()
	data, err := os.ReadFile(process.logFile.Name())
	if err != nil {
		return "read subprocess log: " + err.Error()
	}
	return string(data)
}

func (process *godoxyE2EProcess) stop(t *testing.T) {
	t.Helper()
	var waitErr error
	select {
	case waitErr = <-process.done:
	default:
		_ = process.cmd.Process.Signal(os.Interrupt)
		select {
		case waitErr = <-process.done:
		case <-time.After(5 * time.Second):
			_ = process.cmd.Process.Kill()
			waitErr = <-process.done
			t.Errorf("GoDoxy subprocess did not stop within 5s\n%s", process.logs())
		}
	}
	logs := process.logs()
	_ = process.logFile.Close()
	assert.NoError(t, waitErr, logs)
}
