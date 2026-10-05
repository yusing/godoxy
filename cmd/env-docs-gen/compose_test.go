package main

import (
	"encoding/json/v2"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestComposeEnvironment(t *testing.T) {
	if err := exec.Command("docker", "compose", "version").Run(); err != nil {
		t.Skip("Docker Compose unavailable")
	}
	compose, err := os.ReadFile("../../compose.example.yml")
	if err != nil {
		t.Fatal(err)
	}
	example, _ := render()
	for _, tc := range []struct {
		name, replace, with, endpoint, key string
		fail                               bool
	}{
		{name: "generated example", key: "GODOXY_DOCKER_HOST", endpoint: "tcp://127.0.0.1:2375"},
		{name: "prefixed override", replace: "GODOXY_DOCKER_HOST=tcp://${LISTEN_ADDR}", with: "GODOXY_DOCKER_HOST=unix:///custom/docker.sock", key: "GODOXY_DOCKER_HOST", endpoint: "unix:///custom/docker.sock"},
		{name: "legacy endpoint", replace: "GODOXY_DOCKER_HOST=tcp://${LISTEN_ADDR}", with: "DOCKER_HOST=unix:///custom/docker.sock", key: "DOCKER_HOST", endpoint: "unix:///custom/docker.sock"},
		{name: "missing input", replace: "LISTEN_ADDR=127.0.0.1:2375", with: "", fail: true},
		{name: "empty input", replace: "GODOXY_UID=1000", with: "GODOXY_UID=", fail: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			if err := os.WriteFile(filepath.Join(dir, "compose.yml"), compose, 0600); err != nil {
				t.Fatal(err)
			}
			envText := string(example)
			if tc.replace != "" {
				envText = strings.Replace(envText, tc.replace, tc.with, 1)
			}
			if err := os.WriteFile(filepath.Join(dir, ".env"), []byte(envText), 0600); err != nil {
				t.Fatal(err)
			}
			cmd := exec.CommandContext(t.Context(), "docker", "compose", "--project-directory", dir, "--env-file", filepath.Join(dir, ".env"), "-f", filepath.Join(dir, "compose.yml"), "config", "--format", "json")
			// Inherit no credentials or interpolation overrides from the test process.
			cmd.Env = []string{"PATH=" + os.Getenv("PATH")}
			output, err := cmd.Output()
			if tc.fail {
				if err == nil {
					t.Fatal("missing required input was accepted")
				}
				return
			}
			if err != nil {
				t.Fatal("Compose rejected the generated configuration")
			}
			var config struct {
				Services map[string]struct {
					Environment map[string]string `json:"environment"`
				} `json:"services"`
			}
			if err := json.Unmarshal(output, &config); err != nil {
				t.Fatal("invalid Compose config output")
			}
			app := config.Services["app"].Environment
			if app[tc.key] != tc.endpoint {
				t.Fatal("Compose changed the selected endpoint")
			}
			if tc.key != "DOCKER_HOST" && app["DOCKER_HOST"] != "" {
				t.Fatal("Compose injected an unprefixed endpoint")
			}
		})
	}
}
