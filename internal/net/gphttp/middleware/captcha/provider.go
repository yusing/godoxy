package captcha

import (
	"net/http"
	"time"

	gperr "github.com/yusing/goutils/errs"
)

type Provider interface {
	CSPDirectives() []string
	CSPSources() []string
	Verify(r *http.Request) error
	SessionExpiry() time.Duration
	ScriptHTML() string
	FormHTML() string
}

var ErrCaptchaVerificationFailed = gperr.New("captcha verification failed")
