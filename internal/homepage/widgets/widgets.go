package widgets

import (
	"context"

	"github.com/yusing/go-proxy/internal/gperr"
	"github.com/yusing/go-proxy/internal/serialization"
)

type (
	Config struct {
		Provider string `json:"provider"`
		Config   Widget `json:"config"`
	}
	Widget interface {
		Initialize(ctx context.Context, url string, cfg map[string]any) error
		Data(ctx context.Context) ([]NameValue, error)
	}
	NameValue struct {
		Name  string `json:"name"`
		Value string `json:"value"`
	}
)

const (
	WidgetProviderQbittorrent = "qbittorrent"
)

var widgetProviders = map[string]struct{}{
	WidgetProviderQbittorrent: {},
}

var ErrInvalidProvider = gperr.New("invalid provider")

func (cfg *Config) UnmarshalMap(m map[string]any) error {
	var ok bool
	cfg.Provider, ok = m["provider"].(string)
	if !ok {
		return ErrInvalidProvider.Withf("non string")
	}
	if _, ok := widgetProviders[cfg.Provider]; !ok {
		return ErrInvalidProvider.Subject(cfg.Provider)
	}
	delete(m, "provider")
	m, ok = m["config"].(map[string]any)
	if !ok {
		return gperr.New("invalid config")
	}
	return serialization.MapUnmarshalValidate(m, &cfg.Config)
}
