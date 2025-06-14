package notif

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/yusing/go-proxy/internal/gperr"
	"github.com/yusing/go-proxy/internal/serialization"
)

type (
	Provider interface {
		serialization.CustomValidator

		GetName() string
		GetURL() string
		GetToken() string
		GetMethod() string
		GetMIMEType() string

		MarshalMessage(logMsg *LogMessage) ([]byte, error)
		SetHeaders(logMsg *LogMessage, headers http.Header)

		fmtError(respBody io.Reader) error
	}
	ProviderCreateFunc func(map[string]any) (Provider, gperr.Error)
	ProviderConfig     map[string]any
)

const (
	ProviderGotify  = "gotify"
	ProviderNtfy    = "ntfy"
	ProviderWebhook = "webhook"
)

func (msg *LogMessage) notify(ctx context.Context, provider Provider) error {
	body, err := provider.MarshalMessage(msg)
	if err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(
		ctx,
		http.MethodPost,
		provider.GetURL(),
		bytes.NewReader(body),
	)
	if err != nil {
		return err
	}

	req.Header.Set("Content-Type", provider.GetMIMEType())
	if provider.GetToken() != "" {
		req.Header.Set("Authorization", "Bearer "+provider.GetToken())
	}
	provider.SetHeaders(msg, req.Header)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}

	defer resp.Body.Close()

	switch resp.StatusCode {
	case http.StatusOK, http.StatusCreated, http.StatusAccepted, http.StatusNoContent:
		return nil
	default:
		return fmt.Errorf("http status %d: %w", resp.StatusCode, provider.fmtError(resp.Body))
	}
}
