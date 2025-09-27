//nolint:gofumpt
package route

import (
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"testing"

	expect "github.com/yusing/goutils/testing"
)

func TestPathTraversalAttack(t *testing.T) {
	tmp := t.TempDir()
	root := filepath.Join(tmp, "static")
	if err := os.Mkdir(root, 0o755); err != nil {
		t.Fatalf("Failed to create root directory: %v", err)
	}

	// Create a file inside the root
	validPath := "test.txt"
	validContent := "test content"
	if err := os.WriteFile(filepath.Join(root, validPath), []byte(validContent), 0o644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	// create one at ..
	secretFile := "secret.txt"
	if err := os.WriteFile(filepath.Join(tmp, secretFile), []byte(validContent), 0o644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	traversals := []string{
		"../",
		"./../",
		"./.././",
		"..%2f",
		".%2f..%2f",
		".%2f%2e%2e",
		".%2e",
		".%2e/",
		".%2e%2f",
		"%2e.",
		"%2e%2e",
	}

	for _, traversal := range traversals {
		traversals = append(traversals, "%2f"+traversal)
		traversals = append(traversals, traversal+"%2f")
		traversals = append(traversals, "%2f"+traversal+"%2f")
		traversals = append(traversals, "/"+traversal)
		traversals = append(traversals, traversal+"/")
		traversals = append(traversals, "/"+traversal+"/")
	}

	// Setup the FileServer
	fs, err := NewFileServer(&Route{Root: root})
	if err != nil {
		t.Fatalf("Failed to create FileServer: %v", err)
	}

	// Create a test server with the handler
	ts := httptest.NewServer(fs.handler)
	defer ts.Close()

	// Test valid path
	t.Run("valid path", func(t *testing.T) {
		validURL := ts.URL + "/" + validPath
		resp, err := http.Get(validURL)
		if err != nil {
			t.Errorf("Error making request to %s: %v", validURL, err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			t.Errorf("Expected 200 OK, got %d", resp.StatusCode)
		}

		body, err := io.ReadAll(resp.Body)
		if err != nil {
			t.Errorf("Error reading response body: %v", err)
		}

		if string(body) != validContent {
			t.Errorf("Expected %q, got %q", validContent, string(body))
		}
	})

	// Test ../ path
	// tsURL := Must(url.Parse(ts.URL))
	for _, traversal := range traversals {
		p := traversal + secretFile
		t.Run(p, func(t *testing.T) {
			u := &url.URL{Scheme: "http", Host: ts.Listener.Addr().String(), Path: p}
			resp, err := http.DefaultClient.Do(&http.Request{
				Method: http.MethodGet,
				URL:    u,
			})
			if err != nil {
				t.Errorf("Error making request to %s: %v", p, err)
			}
			defer resp.Body.Close()

			if resp.StatusCode != http.StatusNotFound && resp.StatusCode != http.StatusBadRequest {
				t.Errorf("Expected status 404 or 400, got %d in url %s", resp.StatusCode, u.String())
			}

			u = expect.Must(url.Parse(ts.URL + "/" + p))
			resp, err = http.DefaultClient.Do(&http.Request{
				Method: http.MethodGet,
				URL:    u,
			})
			if err != nil {
				t.Errorf("Error making request to %s: %v", u.String(), err)
			}
			defer resp.Body.Close()
		})
	}
}
