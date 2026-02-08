package captcha

import (
	"errors"
	"net/http"
	"time"
)

type Provider interface {
	CSPDirectives() []string
	CSPSources() []string
	Verify(r *http.Request) error
	SessionExpiry() time.Duration
	ScriptHTML() string
	FormHTML() string
}

var ErrCaptchaVerificationFailed = errors.New("captcha verification failed")
