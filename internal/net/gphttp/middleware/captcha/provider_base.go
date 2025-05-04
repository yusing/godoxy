package captcha

import "time"

type ProviderBase struct {
	Expiry time.Duration `json:"session_expiry"`
}

func (p *ProviderBase) SessionExpiry() time.Duration {
	if p.Expiry == 0 {
		p.Expiry = 24 * time.Hour
	}
	return p.Expiry
}
