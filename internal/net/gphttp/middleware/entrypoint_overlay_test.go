package middleware

import (
	"testing"

	"github.com/stretchr/testify/require"
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
