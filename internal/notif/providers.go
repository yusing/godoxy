package notif

import (
	"context"
	"io"
	"net/http"
	"time"

	"github.com/yusing/go-proxy/internal/gperr"
	gphttp "github.com/yusing/go-proxy/internal/net/gphttp"
	"github.com/yusing/go-proxy/internal/utils"
)

type (
	Provider interface {
		utils.CustomValidator

		GetName() string
		GetURL() string
		GetToken() string
		GetMethod() string
		GetMIMEType() string

		MakeBody(logMsg *LogMessage) (io.Reader, error)
		SetHeaders(logMsg *LogMessage, headers http.Header)

		makeRespError(resp *http.Response) error
	}
	ProviderCreateFunc func(map[string]any) (Provider, gperr.Error)
	ProviderConfig     map[string]any
)

const (
	ProviderGotify  = "gotify"
	ProviderNtfy    = "ntfy"
	ProviderWebhook = "webhook"
)

func notifyProvider(ctx context.Context, provider Provider, msg *LogMessage) error {
	body, err := provider.MakeBody(msg)
	if err != nil {
		return gperr.PrependSubject(provider.GetName(), err)
	}

	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(
		ctx,
		http.MethodPost,
		provider.GetURL(),
		body,
	)
	if err != nil {
		return gperr.PrependSubject(provider.GetName(), err)
	}

	req.Header.Set("Content-Type", provider.GetMIMEType())
	if provider.GetToken() != "" {
		req.Header.Set("Authorization", "Bearer "+provider.GetToken())
	}
	provider.SetHeaders(msg, req.Header)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return gperr.PrependSubject(provider.GetName(), err)
	}

	defer resp.Body.Close()

	if !gphttp.IsSuccess(resp.StatusCode) {
		return provider.makeRespError(resp)
	}
	return nil
}
