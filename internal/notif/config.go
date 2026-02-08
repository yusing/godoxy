package notif

import (
	"errors"
	"strings"

	"github.com/yusing/godoxy/internal/serialization"
	gperr "github.com/yusing/goutils/errs"
)

type NotificationConfig struct {
	ProviderName string   `json:"provider"`
	Provider     Provider `json:"-"`
}

var (
	ErrMissingNotifProvider     = errors.New("missing notification provider")
	ErrInvalidNotifProviderType = errors.New("invalid notification provider type")
	ErrUnknownNotifProvider     = errors.New("unknown notification provider")
)

// UnmarshalMap implements MapUnmarshaler.
func (cfg *NotificationConfig) UnmarshalMap(m map[string]any) (err error) {
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
		return gperr.PrependSubject(ErrUnknownNotifProvider, cfg.ProviderName).
			Withf("expect %s", strings.Join(AvailableProviders, ", "))
	}

	return serialization.MapUnmarshalValidate(m, cfg.Provider)
}
