package captcha

import (
	"crypto/rand"
	"encoding/hex"
	"time"

	_ "embed"

	"github.com/yusing/go-proxy/internal/jsonstore"
	"github.com/yusing/go-proxy/internal/utils"
)

type CaptchaSession struct {
	ID string `json:"id"`

	Expiry time.Time `json:"expiry"`
}

var CaptchaSessions = jsonstore.Store[*CaptchaSession]("captcha_sessions")

func newCaptchaSession(p Provider) *CaptchaSession {
	buf := make([]byte, 32)
	_, _ = rand.Read(buf)
	now := utils.TimeNow()
	return &CaptchaSession{
		ID:     hex.EncodeToString(buf),
		Expiry: now.Add(p.SessionExpiry()),
	}
}

func (s *CaptchaSession) expired() bool {
	return utils.TimeNow().After(s.Expiry)
}
