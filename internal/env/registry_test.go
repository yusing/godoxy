package env

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"
)

func isolate(t *testing.T) {
	t.Helper()
	for _, d := range Definitions() {
		for _, prefix := range []string{"GODOXY_", "GOPROXY_", ""} {
			t.Setenv(prefix+d.Name, "")
			if err := os.Unsetenv(prefix + d.Name); err != nil {
				t.Fatal(err)
			}
		}
	}
	previous := os.Args[0]
	os.Args[0] = "godoxy"
	t.Cleanup(func() { os.Args[0] = previous })
}

func TestRegistryDefaults(t *testing.T) {
	isolate(t)
	seen := map[string]bool{}
	for _, d := range Definitions() {
		if seen[d.Name] {
			t.Fatalf("duplicate definition %s", d.Name)
		}
		seen[d.Name] = true
		if d.Description == "" {
			t.Fatalf("missing description %s", d.Name)
		}
		if d.Compose {
			continue
		}
		switch d.Type {
		case "bool":
			want, err := strconv.ParseBool(d.Default)
			if err != nil || Bool(d.Name) != want {
				t.Errorf("invalid bool default %s", d.Name)
			}
		case "duration":
			want, err := time.ParseDuration(d.Default)
			if err != nil || Duration(d.Name) != want {
				t.Errorf("invalid duration default %s", d.Name)
			}
		case "int":
			want, err := strconv.Atoi(d.Default)
			if err != nil || Int(d.Name) != want {
				t.Errorf("invalid int default %s", d.Name)
			}
		case "address":
			Address(d.Name, "http")
		case "string", "list":
			if String(d.Name) != d.Default {
				t.Errorf("invalid string default %s", d.Name)
			}
		default:
			t.Errorf("unknown type for %s", d.Name)
		}
	}
	if String("DOCKER_HOST") != "unix:///var/run/docker.sock" || Bool("HTTP3_ENABLED") {
		t.Fatal("normal Docker/HTTP3 defaults changed")
	}
}

func TestDerivedDebug(t *testing.T) {
	for _, tc := range []struct {
		name, test, debug, trace, server, websocket string
		binaryTest                                  bool
		want                                        [5]bool
	}{
		{name: "normal"},
		{name: "test env", test: "true", trace: "true", want: [5]bool{true, true, true, false, false}},
		{name: "test binary", binaryTest: true, want: [5]bool{true, true, false, false, false}},
		{name: "test cannot be disabled in test binary", binaryTest: true, test: "false", want: [5]bool{true, true, false, false, false}},
		{name: "explicit debug false", test: "true", debug: "false", trace: "true", want: [5]bool{true, false, false, false, false}},
		{name: "explicit debug true", debug: "true", server: "false", websocket: "false", want: [5]bool{false, true, false, true, true}},
		{name: "individual debug", debug: "false", server: "true", websocket: "true", want: [5]bool{false, false, false, true, true}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			isolate(t)
			if tc.binaryTest {
				os.Args[0] = "godoxy.test"
			}
			for key, val := range map[string]string{"TEST": tc.test, "DEBUG": tc.debug, "TRACE": tc.trace, "SERVER_DEBUG": tc.server, "WEBSOCKET_DEBUG": tc.websocket} {
				t.Setenv("GODOXY_"+key, val)
			}
			for i, key := range []string{"TEST", "DEBUG", "TRACE", "SERVER_DEBUG", "WEBSOCKET_DEBUG"} {
				if Bool(key) != tc.want[i] {
					t.Errorf("unexpected effective %s", key)
				}
			}
		})
	}
}

func TestAliasesAndDiagnostics(t *testing.T) {
	isolate(t)
	for _, tc := range []struct {
		preferred, legacy, plain, source string
		want                             int
	}{
		{"12", "13", "14", "GODOXY_OIDC_RATE_LIMIT", 12},
		{"", "13", "14", "GOPROXY_OIDC_RATE_LIMIT", 13},
		{"", "", "14", "OIDC_RATE_LIMIT", 14},
		{"", "", "", "", 10},
	} {
		t.Setenv("GODOXY_OIDC_RATE_LIMIT", tc.preferred)
		t.Setenv("GOPROXY_OIDC_RATE_LIMIT", tc.legacy)
		t.Setenv("OIDC_RATE_LIMIT", tc.plain)
		if Int("OIDC_RATE_LIMIT") != tc.want {
			t.Fatal("alias precedence mismatch")
		}
		_, source, _ := LookupEnvSource("OIDC_RATE_LIMIT")
		if source != tc.source {
			t.Fatal("source mismatch")
		}
		r := Inspect()
		if (len(r.Deprecated) > 0) != (source == "OIDC_RATE_LIMIT") {
			t.Fatal("deprecation warning does not match selected source")
		}
	}
	// Opaque sentinels are intentionally never printed, even on test failure.
	sensitive := "opaque-sensitive-marker"
	t.Setenv("GODOXY_API_PASSWORD", sensitive)
	t.Setenv("GODOXY_DOCKER_HOST", "tcp://user:"+sensitive+"@docker:2375")
	t.Setenv("GODOXY_OIDC_ISSUER_URL", "https://issuer/?token="+sensitive)
	t.Setenv("GODOXY_TYPO", sensitive)
	t.Setenv("GODOXY_UID", "1000")
	r := Inspect()
	if strings.Contains(fmt.Sprint(r), sensitive) {
		t.Fatal("diagnostics disclosed sensitive input")
	}
	found := false
	for _, key := range r.Unknown {
		if key == "GODOXY_TYPO" {
			found = true
		}
		if key == "GODOXY_UID" {
			t.Fatal("Compose input incorrectly warned")
		}
	}
	if !found {
		t.Fatal("unknown setting not warned")
	}
}

func TestTypedSettings(t *testing.T) {
	isolate(t)
	t.Setenv("GODOXY_INIT_TIMEOUT", "2m30s")
	if Duration("INIT_TIMEOUT") != 150*time.Second {
		t.Fatal("duration override not used")
	}
	t.Setenv("GODOXY_OIDC_SCOPES", "openid, email")
	if fmt.Sprint(CommaSep("OIDC_SCOPES")) != "[openid email]" {
		t.Fatal("list was not trimmed")
	}
	t.Setenv("GODOXY_API_ADDR", "[::1]:9999")
	addr, host, port, _ := Address("API_ADDR", "http")
	if addr != "[::1]:9999" || host != "::1" || port != 9999 {
		t.Fatal("address override not used")
	}
}
