package logging

import (
	"bytes"
	"encoding/json/v2"
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/rs/zerolog"
	godoxy "github.com/yusing/godoxy/internal/env"
)

// These tests deliberately never include captured records or environment values
// in failure messages, even if a regression leaks a credential into the log.
func cleanLoggingEnvironment(t *testing.T) {
	t.Helper()
	for _, entry := range os.Environ() {
		name, _, _ := strings.Cut(entry, "=")
		if strings.HasPrefix(name, "GODOXY_") {
			t.Setenv(name, "")
			if err := os.Unsetenv(name); err != nil {
				t.Fatal("could not isolate prefixed environment")
			}
		}
	}
	for _, d := range godoxy.Definitions() {
		if d.Compose {
			t.Setenv(d.Name, "")
			continue
		}
		for _, prefix := range []string{"GODOXY_", "GOPROXY_", ""} {
			t.Setenv(prefix+d.Name, "")
		}
	}
}

func captureEnvironmentRecords(t *testing.T, forbidden ...string) []map[string]any {
	t.Helper()
	var output bytes.Buffer
	LogEnvironment(zerolog.New(&output).Level(zerolog.InfoLevel))
	for _, value := range forbidden {
		if value != "" && bytes.Contains(output.Bytes(), []byte(value)) {
			t.Fatal("environment diagnostic leaked a protected value")
		}
	}
	var records []map[string]any
	for _, line := range bytes.Split(bytes.TrimSpace(output.Bytes()), []byte("\n")) {
		var record map[string]any
		if err := json.Unmarshal(line, &record); err != nil {
			t.Fatal("environment diagnostic is not a valid JSON record")
		}
		records = append(records, record)
	}
	return records
}

func environmentSetting(t *testing.T, records []map[string]any, name string) map[string]any {
	t.Helper()
	var found map[string]any
	for _, record := range records {
		if record["level"] == "info" && record["name"] == name {
			if found != nil {
				t.Fatalf("duplicate effective setting for %s", name)
			}
			found = record
		}
	}
	if found == nil {
		t.Fatalf("missing effective setting for %s", name)
	}
	return found
}

func TestLogEnvironmentUnknownNamesAndSelectedAliasWarnings(t *testing.T) {
	cleanLoggingEnvironment(t)
	t.Setenv("GODOXY_UNRECOGNIZED_TEST_INPUT", "unknown-private-marker")
	t.Setenv("API_PASSWORD", "alias-private-marker")
	t.Setenv("HTTP_ADDR", ":9191")
	t.Setenv("GODOXY_HTTP_ADDR", ":9292")
	t.Setenv("GOPROXY_HTTPS_ADDR", ":9393")
	t.Setenv("HTTPS_ADDR", ":9494")
	t.Setenv("TAG", "deployment-input-marker")
	t.Setenv("GODOXY_UID", "1000")
	t.Setenv("CUSTOM_CONFIG_INPUT", "custom-private-marker")
	records := captureEnvironmentRecords(t, "unknown-private-marker", "alias-private-marker", "deployment-input-marker", "custom-private-marker")
	warnings := make(map[string]map[string]any)
	for _, record := range records {
		if record["level"] != "warn" {
			continue
		}
		name, ok := record["name"].(string)
		if !ok {
			t.Fatal("warning is missing its variable name")
		}
		if warnings[name] != nil {
			t.Fatalf("duplicate warning for %s", name)
		}
		warnings[name] = record
		for key := range record {
			if key != "level" && key != "name" && key != "replacement" && key != "message" {
				t.Fatal("warning must contain names and explanatory text only")
			}
		}
	}
	if len(warnings) != 2 || warnings["GODOXY_UNRECOGNIZED_TEST_INPUT"] == nil || warnings["API_PASSWORD"] == nil {
		t.Fatal("expected only the unknown-name and selected unprefixed-alias warnings")
	}
	if warnings["API_PASSWORD"]["replacement"] != "GODOXY_API_PASSWORD" {
		t.Fatal("alias warning lacks its migration target")
	}
}

func TestLogEnvironmentRedactsSensitiveSettings(t *testing.T) {
	cleanLoggingEnvironment(t)
	// Pin the protected categories independently of registry metadata so an
	// accidental removal of a Sensitive flag cannot silently weaken this test.
	names := []string{
		"API_JWT_SECRET", "API_USER", "API_PASSWORD", "OIDC_ISSUER_URL",
		"OIDC_CLIENT_ID", "OIDC_CLIENT_SECRET", "OIDC_ALLOWED_USERS",
		"OIDC_ALLOWED_GROUPS", "DOCKER_HOST",
	}
	var forbidden []string
	for _, name := range names {
		value := "private-marker-" + name
		if name == "DOCKER_HOST" || name == "OIDC_ISSUER_URL" {
			value = "https://private-user:private-password@private-host/" + name
		}
		t.Setenv("GODOXY_"+name, value)
		forbidden = append(forbidden, value)
	}
	forbidden = append(forbidden, "private-user", "private-password", "private-host")
	records := captureEnvironmentRecords(t, forbidden...)
	for _, name := range names {
		record := environmentSetting(t, records, "GODOXY_"+name)
		if record["value"] != "[redacted]" || record["source"] != "GODOXY_"+name {
			t.Errorf("sensitive setting not safely reported: %s", name)
		}
	}
}

func TestLogEnvironmentFirstNonemptySource(t *testing.T) {
	for _, tc := range []struct {
		name, current, legacy, bare, source, value string
	}{
		{"current wins", ":9101", ":9102", ":9103", "GODOXY_HTTP_ADDR", ":9101"},
		{"empty current", "", ":9102", ":9103", "GOPROXY_HTTP_ADDR", ":9102"},
		{"empty prefixes", "", "", ":9103", "HTTP_ADDR", ":9103"},
		{"all empty", "", "", "", "default", ":80"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cleanLoggingEnvironment(t)
			t.Setenv("GODOXY_HTTP_ADDR", tc.current)
			t.Setenv("GOPROXY_HTTP_ADDR", tc.legacy)
			t.Setenv("HTTP_ADDR", tc.bare)
			record := environmentSetting(t, captureEnvironmentRecords(t), "GODOXY_HTTP_ADDR")
			if record["source"] != tc.source || record["value"] != tc.value {
				t.Fatal("effective setting does not use the first nonempty source")
			}
		})
	}
}

func TestLogEnvironmentReportsEffectiveTypedSettings(t *testing.T) {
	cleanLoggingEnvironment(t)
	t.Setenv("GODOXY_DEBUG", "false")
	t.Setenv("GODOXY_TRACE", "true")
	t.Setenv("GODOXY_OIDC_RATE_LIMIT", "42")
	t.Setenv("GODOXY_API_JWT_TOKEN_TTL", "90m")
	t.Setenv("GODOXY_FRONTEND_ALIASES", " first, second ")
	records := captureEnvironmentRecords(t)
	for name, value := range map[string]any{
		"GODOXY_DEBUG":             false,
		"GODOXY_TRACE":             false,
		"GODOXY_OIDC_RATE_LIMIT":   float64(42),
		"GODOXY_API_JWT_TOKEN_TTL": "1h30m0s",
		"GODOXY_FRONTEND_ALIASES":  []any{"first", "second"},
	} {
		if !reflect.DeepEqual(environmentSetting(t, records, name)["value"], value) {
			t.Errorf("incorrect effective typed value for %s", name)
		}
	}
	count := 0
	for _, d := range godoxy.Definitions() {
		if !d.Compose {
			count++
			environmentSetting(t, records, "GODOXY_"+d.Name)
		}
	}
	if len(records) != count {
		t.Fatal("effective report must include server settings only")
	}
}
