package middleware

import (
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yusing/godoxy/internal/route/routes"
	"github.com/yusing/godoxy/internal/route/rules"
)

func TestBuildEntrypointRouteOverlayReturnsSentinelWhenNoPromotionOccurs(t *testing.T) {
	t.Run("no_matching_entrypoint_middleware", func(t *testing.T) {
		overlay, err := BuildEntrypointRouteOverlay(
			"entrypoint",
			[]map[string]any{{
				"use": "response",
			}},
			"test-route",
			map[string]OptionsRaw{
				"redirectHTTP": {
					"bypass": []string{"path /health"},
				},
			},
		)

		require.Nil(t, overlay)
		require.ErrorIs(t, err, ErrNoEntrypointRouteOverlay)
	})

	t.Run("empty_route_middlewares", func(t *testing.T) {
		overlay, err := BuildEntrypointRouteOverlay(
			"entrypoint",
			[]map[string]any{{
				"use": "response",
			}},
			"test-route",
			nil,
		)

		require.Nil(t, overlay)
		require.ErrorIs(t, err, ErrNoEntrypointRouteOverlay)
	})
}

func TestBuildEntrypointRouteOverlayPromotesRouteBypass(t *testing.T) {
	overlay, err := BuildEntrypointRouteOverlay(
		"entrypoint",
		[]map[string]any{{
			"use": "redirectHTTP",
		}},
		"test-route",
		map[string]OptionsRaw{
			"redirectHTTP": {
				"bypass": []string{"path /health"},
			},
		},
	)

	require.NoError(t, err)
	require.NotNil(t, overlay)
	require.NotNil(t, overlay.Middleware)
	require.Contains(t, overlay.ConsumedBypass, "redirecthttp")
	require.Contains(t, overlay.ConsumedMiddlewares, "redirecthttp")
}

func TestBuildEntrypointRouteOverlayKeepsNonBypassRouteMiddlewareActive(t *testing.T) {
	overlay, err := BuildEntrypointRouteOverlay(
		"entrypoint",
		[]map[string]any{{
			"use": "redirectHTTP",
		}},
		"test-route",
		map[string]OptionsRaw{
			"redirectHTTP": {
				"bypass":       []string{"path /health"},
				"redirectHTTP": "https://example.com",
			},
		},
	)

	require.NoError(t, err)
	require.NotNil(t, overlay)
	require.NotNil(t, overlay.Middleware)
	require.Contains(t, overlay.ConsumedBypass, "redirecthttp")
	require.Empty(t, overlay.ConsumedMiddlewares)
}

func TestQualifyBypassWithRoutePreservesCompositeRuleSemantics(t *testing.T) {
	var composite rules.RuleOn
	require.NoError(t, composite.Parse("path /health | path /status"))

	qualified, err := qualifyBypassWithRoute("test-route", Bypass{composite})
	require.NoError(t, err)
	require.Len(t, qualified, 1)

	matches := func(path, routeName string) bool {
		req := httptest.NewRequest("GET", "http://example.com"+path, nil)
		if routeName != "" {
			req = routes.WithRouteContext(req, fakeMiddlewareHTTPRoute{name: routeName})
		}
		return qualified[0].Check(httptest.NewRecorder(), req)
	}

	require.True(t, matches("/health", "test-route"))
	require.True(t, matches("/status", "test-route"))
	require.False(t, matches("/health", "other-route"))
	require.False(t, matches("/metrics", "test-route"))
}
