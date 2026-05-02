package captcha

import (
	"bytes"
	"context"
	"errors"
	"net"
	"net/http"
	"net/url"
	"time"

	_ "embed"

	gperr "github.com/yusing/goutils/errs"
	strutils "github.com/yusing/goutils/strings"
)

type HcaptchaProvider struct {
	ProviderBase

	SiteKey strutils.Redacted `json:"site_key" validate:"required"`
	Secret  strutils.Redacted `json:"secret" validate:"required"`
}

// CSPDirectives returns the CSP directives for the Hcaptcha provider.
// See: https://docs.hcaptcha.com/#content-security-policy-settings
func (p *HcaptchaProvider) CSPDirectives() []string {
	return []string{"script-src", "frame-src", "style-src", "connect-src"}
}

// CSPSources returns the CSP sources for the Hcaptcha provider.
// See: https://docs.hcaptcha.com/#content-security-policy-settings
func (p *HcaptchaProvider) CSPSources() []string {
	return []string{
		"https://hcaptcha.com",
		"https://*.hcaptcha.com",
	}
}

func (p *HcaptchaProvider) Verify(r *http.Request) error {
	response := r.PostFormValue("h-captcha-response")
	if response == "" {
		return errors.New("h-captcha-response is missing")
	}

	remoteIP := r.RemoteAddr
	if ip, _, err := net.SplitHostPort(remoteIP); err == nil {
		remoteIP = ip
	}

	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
	defer cancel()
	formData := url.Values{}
	formData.Set("secret", p.Secret.String())
	formData.Set("response", response)
	formData.Set("remoteip", remoteIP)
	formData.Set("sitekey", p.SiteKey.String())

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://api.hcaptcha.com/siteverify", bytes.NewBufferString(formData.Encode()))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	var respData struct {
		Success bool     `json:"success"`
		Error   []string `json:"error-codes"`
	}
	if err := strutils.NewJSONDecoder(resp.Body).Decode(&respData); err != nil {
		return err
	}

	if !respData.Success {
		return gperr.JoinLines(ErrCaptchaVerificationFailed, respData.Error...)
	}

	return nil
}

func (p *HcaptchaProvider) ScriptHTML() string {
	return `
<script src="https://js.hcaptcha.com/1/api.js" async defer></script>`
}

func (p *HcaptchaProvider) FormHTML() string {
	return `
<div
	class="h-captcha"
	data-sitekey="` + p.SiteKey.String() + `"
	data-callback="onDataCallback"
/>`
}
