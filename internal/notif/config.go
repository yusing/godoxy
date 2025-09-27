package notif

import (
	"github.com/yusing/godoxy/internal/serialization"
	gperr "github.com/yusing/goutils/errs"
)

type NotificationConfig struct {
	ProviderName string   `json:"provider"`
	Provider     Provider `json:"-"`
}

var (
	ErrMissingNotifProvider     = gperr.New("missing notification provider")
	ErrInvalidNotifProviderType = gperr.New("invalid notification provider type")
	ErrUnknownNotifProvider     = gperr.New("unknown notification provider")
)

// UnmarshalMap implements MapUnmarshaler.
func (cfg *NotificationConfig) UnmarshalMap(m map[string]any) (err gperr.Error) {
	// extract provider name
	providerName := m["provider"]
	switch providerName := providerName.(type) {
	case string:
		cfg.ProviderName = providerName
	default:
		return ErrInvalidNotifProviderType
	}
	delete(m, "provider")

	if cfg.ProviderName == "" {
		return ErrMissingNotifProvider
	}

	// validate provider name and initialize provider
	switch cfg.ProviderName {
	case ProviderWebhook:
		cfg.Provider = &Webhook{}
	case ProviderGotify:
		cfg.Provider = &GotifyClient{}
	case ProviderNtfy:
		cfg.Provider = &Ntfy{}
	default:
		return ErrUnknownNotifProvider.
			Subject(cfg.ProviderName).
			Withf("expect %s or %s", ProviderWebhook, ProviderGotify)
	}

	return serialization.MapUnmarshalValidate(m, cfg.Provider)
}
