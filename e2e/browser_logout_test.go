package e2e

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/tls"
	json "encoding/json/v2"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"net/http/httputil"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/yusing/godoxy/internal/testcert"
)

const (
	logoutUser     = "e2e-user"
	logoutPassword = "e2e-password"
)

func TestBrowserLogout(t *testing.T) {
	if os.Getenv("GODOXY_E2E_BROWSER") != "1" {
		t.Skip("run shadowtree test-browser")
	}

	binary := filepath.Join(t.TempDir(), "godoxy")
	ctx, cancel := context.WithTimeout(t.Context(), 3*time.Minute)
	defer cancel()
	build := exec.CommandContext(ctx, "go", "build", "-tags", "production",
		"-ldflags=-checklinkname=0", "-o", binary, "./cmd",
	)
	build.Dir = ".."
	output, err := build.CombinedOutput()
	require.NoError(t, err, "build GoDoxy: %s", output)

	t.Run("PocketID", func(t *testing.T) {
		providerURL, loginToken, registerClient := startLogoutPocketID(t)
		appURL := startLogoutGoDoxy(t, binary, true, func(appURL string) []string {
			clientID, secret := registerClient(appURL)
			return []string{
				"GODOXY_OIDC_ISSUER_URL=" + providerURL,
				"GODOXY_OIDC_CLIENT_ID=" + clientID,
				"GODOXY_OIDC_CLIENT_SECRET=" + secret,
				"GODOXY_OIDC_ALLOWED_USERS=" + logoutUser,
			}
		})

		browser := newLogoutBrowser(t)
		browser.run("open", providerURL)
		// Exchange a fixture login code to establish the real IdP session
		// without requiring a physical passkey in the browser.
		browser.loginToPocketID(loginToken)
		require.Equal(t, "200", browser.eval(
			`fetch("/api/users/me").then(response => response.status)`,
		))
		browser.run("open", appURL)
		browser.waitForLogoutButton(appURL)
		require.Equal(t, "true", browser.eval(`(() => {
			document.cookie = "godoxy_csrf=; Max-Age=0; Path=/";
			return !document.cookie.includes("godoxy_csrf=");
		})()`))

		browser.clickLogout()

		// With the registered callback, GoDoxy sends the signed-out browser
		// to a fresh OIDC interaction, not the provider's logout page.
		browser.run("wait", "--fn", fmt.Sprintf(
			`location.origin === %q && location.pathname === "/interaction" &&
			document.querySelector("h1")?.textContent.includes("Sign in to GoDoxy")`,
			providerURL,
		))
		require.Equal(t, "401", browser.eval(
			`fetch("/api/users/me").then(response => response.status)`,
		))
	})

	t.Run("UsernamePassword", func(t *testing.T) {
		appURL := startLogoutGoDoxy(t, binary, false, func(string) []string {
			return []string{
				"GODOXY_API_USER=" + logoutUser,
				"GODOXY_API_PASSWORD=" + logoutPassword,
			}
		})

		browser := newLogoutBrowser(t)
		browser.run("open", appURL)
		waitForLogin := func() {
			browser.run("wait", "--fn", `location.pathname === "/login" &&
				Boolean(document.querySelector('input[type="password"]'))`)
		}
		waitForLogin()
		browser.loginWithPassword()
		browser.run("open", appURL)
		browser.waitForLogoutButton(appURL)
		require.Equal(t, "true", browser.eval(`(() => {
			document.cookie = "godoxy_csrf=stale; Path=/; SameSite=Strict";
			return document.cookie.split("; ").includes("godoxy_csrf=stale");
		})()`))

		browser.clickLogout()
		waitForLogin()
		browser.run("open", appURL)
		waitForLogin()
	})
}

func startLogoutGoDoxy(
	t *testing.T, binary string, https bool, authEnv func(appURL string) []string,
) string {
	t.Helper()
	fixture := t.TempDir()
	config := "webui:\n  aliases: [localhost]\n"
	if https {
		cert := testcert.WriteFiles(t, fixture)
		config += fmt.Sprintf(
			"autocert:\n  provider: local\n  cert_path: %q\n  key_path: %q\n",
			cert.CertPath, cert.KeyPath,
		)
	}
	configDir := filepath.Join(fixture, "config")
	require.NoError(t, os.Mkdir(configDir, 0o700))
	require.NoError(t, os.WriteFile(
		filepath.Join(configDir, "config.yml"), []byte(config), 0o600))

	scheme, bindAddr := "http", "127.0.0.1:0"
	transport := &http.Transport{}
	if https {
		scheme, bindAddr = "https", "[::1]:0"
		// Like the browser, accept the disposable loopback certificate.
		transport.TLSClientConfig = &tls.Config{InsecureSkipVerify: true}
	}
	t.Cleanup(transport.CloseIdleConnections)
	client := &http.Client{Transport: transport, Timeout: 500 * time.Millisecond}
	for range 3 {
		listener, err := net.Listen("tcp", bindAddr)
		require.NoError(t, err)
		addr := listener.Addr().(*net.TCPAddr)
		appURL := scheme + "://localhost:" + strconv.Itoa(addr.Port)
		httpAddr, httpsAddr := addr.String(), ""
		if https {
			httpAddr, httpsAddr = "127.0.0.1:0", addr.String()
		}
		// Keep the user-facing port occupied while registering its callbacks.
		env := func() []string {
			defer listener.Close()
			return append([]string{
				"GODOXY_HTTP_ADDR=" + httpAddr,
				"GODOXY_HTTPS_ADDR=" + httpsAddr,
				"GODOXY_API_ADDR=127.0.0.1:0",
				"GODOXY_API_JWT_SECURE=" + strconv.FormatBool(https),
			}, authEnv(appURL)...)
		}()
		if tryLogoutGoDoxy(t, binary, fixture, env, client, appURL) {
			return appURL
		}
	}
	t.Fatal("GoDoxy fixture could not bind a loopback port after 3 attempts")
	return ""
}

func tryLogoutGoDoxy(
	t *testing.T, binary, fixture string, env []string, client *http.Client, appURL string,
) bool {
	t.Helper()
	cmd := exec.Command(binary)
	cmd.Dir = fixture
	cmd.Env = env
	log, err := os.Create(filepath.Join(fixture, "godoxy.log"))
	require.NoError(t, err)
	defer log.Close()
	cmd.Stdout = log
	cmd.Stderr = log
	require.NoError(t, cmd.Start())
	done := make(chan struct{})
	var exitErr error
	go func() {
		exitErr = cmd.Wait()
		close(done)
	}()
	stop := func() error {
		select {
		case <-done:
			return exitErr
		default:
			_ = cmd.Process.Signal(os.Interrupt)
		}
		select {
		case <-done:
			return exitErr
		case <-time.After(5 * time.Second):
			_ = cmd.Process.Kill()
			<-done
			return errors.New("GoDoxy fixture did not stop")
		}
	}
	err = waitForLogoutService(t.Context(), client, appURL+"/api/v1/version", done)
	if err != nil {
		exitErr := stop()
		data, readErr := os.ReadFile(log.Name())
		require.NoError(t, readErr)
		if bytes.Contains(data, []byte("bind: address already in use")) {
			t.Logf("GoDoxy fixture lost port %s; retrying", appURL)
			return false
		}
		t.Fatalf("GoDoxy fixture startup: %v (process: %v)\n%s", err, exitErr, data)
	}
	t.Cleanup(func() {
		if err := stop(); err != nil {
			t.Errorf("GoDoxy fixture exited: %v", err)
		}
		if t.Failed() {
			data, err := os.ReadFile(log.Name())
			require.NoError(t, err)
			t.Logf("GoDoxy fixture:\n%s", data)
		}
	})
	return true
}

func waitForLogoutService(
	ctx context.Context, client *http.Client, target string, done <-chan struct{},
) error {
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		return err
	}
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	for {
		resp, err := client.Do(req)
		if err == nil {
			_ = resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				select {
				case <-done:
					return errors.New("service exited")
				default:
					return nil
				}
			}
		}
		select {
		case <-done:
			return errors.New("service exited")
		case <-ctx.Done():
			return fmt.Errorf("waiting for %s: %w", target, ctx.Err())
		case <-ticker.C:
		}
	}
}

func startLogoutPocketID(t *testing.T) (
	providerURL, loginToken string, registerClient func(appURL string) (id, secret string),
) {
	t.Helper()
	const image = "ghcr.io/pocket-id/pocket-id:v2.19.3"
	apiKey := rand.Text()
	name := "godoxy-logout-" + strings.ToLower(rand.Text())
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		if t.Failed() {
			// Discovery diagnostics do not include browser authorization URLs.
			log, _ := exec.CommandContext(ctx, "docker", "inspect", "--format",
				"{{.State.Status}} exit={{.State.ExitCode}} {{.State.Error}}",
				name).Output()
			t.Logf("Pocket ID fixture: %s", log)
		}
		output, err := exec.CommandContext(ctx, "docker", "rm", "-f", "-v",
			name).CombinedOutput()
		if !bytes.Contains(output, []byte("No such container: "+name)) {
			require.NoError(t, err, "remove Pocket ID fixture: %s", output)
		}
	})

	// Pocket ID needs its public URL before Docker assigns the upstream port.
	// Keep the public listener open so neither port needs a reserve/rebind.
	proxy := httptest.NewUnstartedServer(nil)
	t.Cleanup(proxy.Close)
	port := proxy.Listener.Addr().(*net.TCPAddr).Port
	providerURL = "http://localhost:" + strconv.Itoa(port)

	ctx, cancel := context.WithTimeout(t.Context(), 3*time.Minute)
	defer cancel()
	cmd := exec.CommandContext(ctx, "docker", "create", "--name", name,
		"--publish", "127.0.0.1::1411",
		"--env", "APP_URL="+providerURL, "--env", "ANALYTICS_DISABLED=true",
		"--env", "STATIC_API_KEY", "--env", "ENCRYPTION_KEY", image,
	)
	cmd.Env = append(os.Environ(),
		"STATIC_API_KEY="+apiKey, "ENCRYPTION_KEY="+rand.Text()+rand.Text(),
	)
	output, err := cmd.CombinedOutput()
	require.NoError(t, err, "create Pocket ID fixture: %s", output)
	output, err = exec.CommandContext(ctx, "docker", "start", name).CombinedOutput()
	require.NoError(t, err, "start Pocket ID fixture: %s", output)
	output, err = exec.CommandContext(ctx, "docker", "port", name, "1411/tcp").CombinedOutput()
	require.NoError(t, err, "read Pocket ID published address: %s", output)
	target, err := url.Parse("http://" + strings.TrimSpace(string(output)))
	require.NoError(t, err)
	proxy.Config.Handler = httputil.NewSingleHostReverseProxy(target)
	proxy.Start()

	require.NoError(t, waitForLogoutService(t.Context(), &http.Client{
		Timeout: 500 * time.Millisecond,
	}, providerURL+"/.well-known/openid-configuration", nil))
	client := &http.Client{Timeout: 3 * time.Second}
	post := func(path string, body map[string]any, result any) {
		t.Helper()
		input, err := json.Marshal(body)
		require.NoError(t, err)
		req, err := http.NewRequestWithContext(t.Context(), http.MethodPost,
			providerURL+path, bytes.NewReader(input))
		require.NoError(t, err)
		req.Header.Set("X-API-KEY", apiKey)
		req.Header.Set("Content-Type", "application/json")
		resp, err := client.Do(req)
		require.NoError(t, err)
		defer resp.Body.Close()
		require.Equal(t, http.StatusCreated, resp.StatusCode, "Pocket ID %s", path)
		if result != nil {
			require.NoError(t, json.UnmarshalRead(resp.Body, result))
		}
	}

	var user struct {
		ID string `json:"id"`
	}
	post("/api/users", map[string]any{
		"username": logoutUser, "email": "logout@example.invalid",
	}, &user)
	require.NotEmpty(t, user.ID)
	var access struct {
		Token string `json:"token"`
	}
	post("/api/users/"+user.ID+"/one-time-access-token",
		map[string]any{"ttl": "5m"}, &access)
	require.NotEmpty(t, access.Token)
	return providerURL, access.Token, func(appURL string) (string, string) {
		clientID := strings.ToLower(rand.Text())
		post("/api/oidc/clients", map[string]any{
			"id": clientID, "name": "GoDoxy", "skipConsent": true,
			"callbackURLs":       []string{appURL + "/auth/callback"},
			"logoutCallbackURLs": []string{appURL},
			"isGroupRestricted":  false,
		}, nil)
		var credentials struct {
			Secret string `json:"secret"`
		}
		post("/api/oidc/clients/"+clientID+"/secrets",
			map[string]any{}, &credentials)
		require.NotEmpty(t, credentials.Secret)
		return clientID, credentials.Secret
	}
}

type logoutBrowser struct {
	t    *testing.T
	args []string
}

func newLogoutBrowser(t *testing.T) logoutBrowser {
	t.Helper()
	browser := logoutBrowser{t, []string{
		"--session", "godoxy-logout-" + strings.ToLower(rand.Text()),
		"--ignore-https-errors",
	}}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		cmd := exec.CommandContext(ctx, "agent-browser",
			append(browser.args, "close")...)
		require.NoError(t, cmd.Run(), "close the Chromium fixture")
	})
	return browser
}

func (b logoutBrowser) run(args ...string) string {
	b.t.Helper()
	return b.command("", args...)
}

func (b logoutBrowser) eval(script string) string {
	b.t.Helper()
	return b.command(script, "eval", "--stdin")
}

func (b logoutBrowser) command(input string, args ...string) string {
	b.t.Helper()
	ctx, cancel := context.WithTimeout(b.t.Context(), 35*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "agent-browser", append(b.args, args...)...)
	cmd.Stdin = strings.NewReader(input)
	result, err := cmd.CombinedOutput()
	if input != "" {
		// Script input can contain a fixture login code.
		require.NoError(b.t, err, "browser script failed")
	} else {
		require.NoError(b.t, err, "browser %s: %s", args[0], result)
	}
	return strings.TrimSpace(string(result))
}

func (b logoutBrowser) loginToPocketID(token string) {
	b.t.Helper()
	// Pocket ID v2.19.3 exchanges the login code at this path.
	require.Equal(b.t, "200", b.eval(fmt.Sprintf(`
		fetch(%q, { method: "POST" }).then(response => response.status)
	`, "/api/one-time-access-token/"+token)))
}

func (b logoutBrowser) loginWithPassword() {
	b.t.Helper()
	body, err := json.Marshal(map[string]string{
		"username": logoutUser, "password": logoutPassword,
	})
	require.NoError(b.t, err)
	require.Equal(b.t, "200", b.eval(fmt.Sprintf(`
		fetch("/api/v1/auth/callback", {
			method: "POST",
			headers: {"Content-Type": "application/json"},
			body: %q,
		}).then(response => response.status)
	`, body)))
}

func (b logoutBrowser) waitForLogoutButton(appURL string) {
	b.t.Helper()
	b.run("wait", "--load", "load")
	b.run("wait", "--fn", fmt.Sprintf(`location.origin === %q &&
		location.pathname === "/" &&
		Boolean(document.querySelector('button[aria-label="Logout"]'))`, appURL))
}

func (b logoutBrowser) clickLogout() {
	b.t.Helper()
	b.run("find", "role", "button", "click", "--name", "Logout")
}
