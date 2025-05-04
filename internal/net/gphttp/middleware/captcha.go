package middleware

import (
	"net/http"

	"github.com/yusing/go-proxy/internal/net/gphttp/middleware/captcha"
)

type hCaptcha struct {
	captcha.HcaptchaProvider
}

func (h *hCaptcha) before(w http.ResponseWriter, r *http.Request) (proceed bool) {
	return captcha.PreRequest(h, w, r)
}

var HCaptcha = NewMiddleware[hCaptcha]()
