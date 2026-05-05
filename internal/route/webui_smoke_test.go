package route

import (
	"io/fs"
	"net/http"
	"net/http/httptest"
	"path"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yusing/godoxy/internal/route/rules"
	rulepresets "github.com/yusing/godoxy/internal/route/rules/presets"
	"github.com/yusing/godoxy/internal/types"
	"github.com/yusing/godoxy/webui"
)

func TestEmbeddedWebUIRouteSmoke(t *testing.T) {
	prevAPI, _ := rules.GetHandler("api")
	prevAuth := rules.GetAuthHandler()
	t.Cleanup(func() {
		rules.ReplaceHandler("api", prevAPI)
		rules.InitAuthHandler(prevAuth)
	})

	apiStub := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/version":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`"test-version"`))
		case "/api/v1/auth/logout":
			http.Redirect(w, r, "/", http.StatusFound)
		default:
			http.NotFound(w, r)
		}
	})
	rules.ReplaceHandler("api", apiStub)

	rules.InitAuthHandler(func(http.ResponseWriter, *http.Request) bool { return true })

	webuiRules, ok := rulepresets.GetRulePreset("webui.yml")
	require.True(t, ok)

	fileServer, err := NewFileServer(&Route{
		Root:     "embed://webui",
		Metadata: Metadata{RootFS: webui.Dist()},
		SPA:      true,
		Index:    "_shell.html",
		Rules:    webuiRules,
	})
	require.NoError(t, err)

	underscoredBundlePath := findUnderscoredBundleAssetPath(t)

	tests := []struct {
		name       string
		path       string
		wantStatus int
		wantBody   string
		wantHeader map[string]string
		useRules   bool
	}{
		{
			name:       "login page",
			path:       "/login",
			wantStatus: http.StatusOK,
			wantBody:   "<!DOCTYPE html>",
			useRules:   true,
		},
		{
			name:       "version API proxy",
			path:       "/api/v1/version",
			wantStatus: http.StatusOK,
			wantBody:   "\"",
			useRules:   true,
		},
		{
			name:       "legacy auth proxy rewrite",
			path:       "/auth/logout",
			wantStatus: http.StatusFound,
			wantBody:   "<a href=\"/\">Found</a>.",
			useRules:   true,
		},
		{
			name:       "SPA fallback",
			path:       "/routes",
			wantStatus: http.StatusOK,
			wantBody:   "<!DOCTYPE html>",
		},
		{
			name:       "static asset cache header",
			path:       "/icon0.svg",
			wantStatus: http.StatusOK,
			wantBody:   "<svg",
			useRules:   true,
		},
		{
			name:       "underscored bundle asset embedded",
			path:       underscoredBundlePath,
			wantStatus: http.StatusOK,
			wantBody:   "import",
			useRules:   true,
		},
	}

	rulesHandler := webuiRules.BuildHandler(fileServer.handler.ServeHTTP)
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodGet, tt.path, nil)

			if tt.useRules {
				rulesHandler.ServeHTTP(rec, req)
			} else {
				fileServer.handler.ServeHTTP(rec, req)
			}

			require.Equal(t, tt.wantStatus, rec.Code)
			require.Contains(t, rec.Body.String(), tt.wantBody)
			for key, value := range tt.wantHeader {
				require.Equal(t, value, rec.Header().Get(key))
			}
		})
	}
}

func TestEmbeddedWebUIRouteMiddlewaresWrapRules(t *testing.T) {
	prevAPI, _ := rules.GetHandler("api")
	prevAuth := rules.GetAuthHandler()
	t.Cleanup(func() {
		rules.ReplaceHandler("api", prevAPI)
		rules.InitAuthHandler(prevAuth)
	})

	rules.ReplaceHandler("api", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("api response"))
	}))
	rules.InitAuthHandler(func(http.ResponseWriter, *http.Request) bool { return true })

	webuiRules, ok := rulepresets.GetRulePreset("webui.yml")
	require.True(t, ok)

	fileServer, err := NewFileServer(&Route{
		Alias:    "godoxy",
		Root:     "embed://webui",
		Metadata: Metadata{RootFS: webui.Dist()},
		SPA:      true,
		Index:    "_shell.html",
		Rules:    webuiRules,
		Middlewares: map[string]types.LabelMap{
			"themed": {
				"css": "https://css.tyleo.dev/gdx/global.css",
			},
			"response": {
				"set_headers": map[string]any{
					"X-Test-Header": "test-value",
				},
			},
		},
	})
	require.NoError(t, err)
	require.NoError(t, fileServer.prepareHandler())

	tests := []struct {
		name        string
		path        string
		wantBody    string
		wantHeader  string
		wantNoTheme bool
	}{
		{
			name:       "spa html gets response header and theme",
			path:       "/routes",
			wantBody:   `href="https://css.tyleo.dev/gdx/global.css"`,
			wantHeader: "test-value",
		},
		{
			name:        "api handled by rules still gets response header",
			path:        "/api/v1/version",
			wantBody:    "api response",
			wantHeader:  "test-value",
			wantNoTheme: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodGet, tt.path, nil)

			fileServer.handler.ServeHTTP(rec, req)

			require.Equal(t, http.StatusOK, rec.Code)
			require.Equal(t, tt.wantHeader, rec.Header().Get("X-Test-Header"))
			require.Contains(t, rec.Body.String(), tt.wantBody)
			if tt.wantNoTheme {
				require.NotContains(t, rec.Body.String(), "css.tyleo.dev")
			}
		})
	}
}

// findUnderscoredBundleAssetPath returns an HTTP path like "/assets/_-<hash>.js" by scanning the
// same embedded FS used at runtime (Vite chunk hashes change between builds).
func findUnderscoredBundleAssetPath(t *testing.T) string {
	t.Helper()
	var found string
	err := fs.WalkDir(webui.Dist(), ".", func(p string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() || path.Dir(p) != "assets" {
			return nil
		}
		base := path.Base(p)
		if strings.HasPrefix(base, "_-") && strings.HasSuffix(base, ".js") {
			found = p
			return fs.SkipAll
		}
		return nil
	})
	require.NoError(t, err)
	require.NotEmpty(t, found, "expected embedded assets/_-*.js")
	return "/" + found
}
