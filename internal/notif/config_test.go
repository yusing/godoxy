package notif

import (
	"net/http"
	"testing"

	"github.com/yusing/go-proxy/internal/utils"
	expect "github.com/yusing/go-proxy/internal/utils/testing"
)

func TestNotificationConfig(t *testing.T) {
	tests := []struct {
		name     string
		cfg      map[string]any
		expected Provider
		wantErr  bool
	}{
		{
			name: "valid_webhook",
			cfg: map[string]any{
				"name":     "test",
				"provider": "webhook",
				"template": "discord",
				"url":      "https://example.com",
			},
			expected: &Webhook{
				ProviderBase: ProviderBase{
					Name: "test",
					URL:  "https://example.com",
				},
				Template:  "discord",
				Method:    http.MethodPost,
				MIMEType:  "application/json",
				ColorMode: "dec",
				Payload:   discordPayload,
			},
			wantErr: false,
		},
		{
			name: "valid_gotify",
			cfg: map[string]any{
				"name":     "test",
				"provider": "gotify",
				"url":      "https://example.com",
				"token":    "token",
			},
			expected: &GotifyClient{
				ProviderBase: ProviderBase{
					Name:  "test",
					URL:   "https://example.com",
					Token: "token",
				},
			},
			wantErr: false,
		},
		{
			name: "invalid_provider",
			cfg: map[string]any{
				"name":     "test",
				"provider": "invalid",
				"url":      "https://example.com",
			},
			wantErr: true,
		},
		{
			name: "missing_url",
			cfg: map[string]any{
				"name":     "test",
				"provider": "webhook",
			},
			wantErr: true,
		},
		{
			name: "missing_provider",
			cfg: map[string]any{
				"name":     "test",
				"provider": "webhook",
			},
			wantErr: true,
		},
		{
			name: "gotify_missing_token",
			cfg: map[string]any{
				"name":     "test",
				"provider": "gotify",
				"url":      "https://example.com",
			},
			wantErr: true,
		},
		{
			name: "webhook_missing_payload",
			cfg: map[string]any{
				"name":     "test",
				"provider": "webhook",
				"url":      "https://example.com",
			},
			wantErr: true,
		},
		{
			name: "webhook_missing_url",
			cfg: map[string]any{
				"name":     "test",
				"provider": "webhook",
			},
			wantErr: true,
		},
		{
			name: "webhook_invalid_template",
			cfg: map[string]any{
				"name":     "test",
				"provider": "webhook",
				"url":      "https://example.com",
				"template": "invalid",
			},
			wantErr: true,
		},
		{
			name: "webhook_invalid_json_payload",
			cfg: map[string]any{
				"name":      "test",
				"provider":  "webhook",
				"url":       "https://example.com",
				"mime_type": "application/json",
				"payload":   "invalid",
			},
			wantErr: true,
		},
		{
			name: "webhook_empty_text_payload",
			cfg: map[string]any{
				"name":      "test",
				"provider":  "webhook",
				"url":       "https://example.com",
				"mime_type": "text/plain",
			},
			wantErr: true,
		},
		{
			name: "webhook_invalid_method",
			cfg: map[string]any{
				"name":     "test",
				"provider": "webhook",
				"url":      "https://example.com",
				"method":   "invalid",
			},
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var cfg NotificationConfig
			provider := tt.cfg["provider"]
			err := utils.MapUnmarshalValidate(tt.cfg, &cfg)
			if tt.wantErr {
				expect.HasError(t, err)
			} else {
				expect.NoError(t, err)
				expect.Equal(t, provider.(string), cfg.ProviderName)
				expect.Equal(t, cfg.Provider, tt.expected)
			}
		})
	}
}
