package notif

import (
	_ "embed"
	"errors"
	"io"
	"net/http"
	"strings"

	"github.com/bytedance/sonic"
	gperr "github.com/yusing/goutils/errs"
)

type Webhook struct {
	ProviderBase
	Template  string `json:"template"`
	Payload   string `json:"payload"`
	Method    string `json:"method"`
	MIMEType  string `json:"mime_type"`
	ColorMode string `json:"color_mode"`
}

//go:embed templates/discord.json
var discordPayload string

var webhookTemplates = map[string]string{
	"discord": discordPayload,
}

func (webhook *Webhook) Validate() error {
	var errs gperr.Builder

	if err := webhook.ProviderBase.Validate(); err != nil && !errors.Is(err, ErrMissingToken) {
		errs.Add(err)
	}

	switch webhook.MIMEType {
	case "":
		webhook.MIMEType = MimeTypeJSON
	case MimeTypeJSON, MimeTypeForm, MimeTypeText:
	default:
		errs.Addf("invalid mime_type, expect %s", strings.Join([]string{"empty", MimeTypeJSON, MimeTypeForm, MimeTypeText}, ", "))
	}

	switch webhook.Template {
	case "":
		if webhook.MIMEType == MimeTypeJSON {
			if !validateJSONPayload(webhook.Payload) {
				errs.Adds("invalid payload, expect valid JSON")
			}
		}
		if webhook.Payload == "" {
			errs.Adds("invalid payload, expect non-empty")
		}
	case "discord":
		webhook.ColorMode = "dec"
		webhook.Method = http.MethodPost
		webhook.MIMEType = MimeTypeJSON
		if webhook.Payload == "" {
			webhook.Payload = discordPayload
		}
	default:
		errs.Adds("invalid template, expect empty or 'discord'")
	}

	switch webhook.Method {
	case "":
		webhook.Method = http.MethodPost
	case http.MethodGet, http.MethodPost, http.MethodPut:
	default:
		errs.Adds("invalid method, expect empty, 'GET', 'POST' or 'PUT'")
	}

	switch webhook.ColorMode {
	case "":
		webhook.ColorMode = "hex"
	case "hex", "dec":
	default:
		errs.Adds("invalid color_mode, expect empty, 'hex' or 'dec'")
	}

	return errs.Error()
}

// GetMethod implements Provider.
func (webhook *Webhook) GetMethod() string {
	return webhook.Method
}

// GetMIMEType implements Provider.
func (webhook *Webhook) GetMIMEType() string {
	return webhook.MIMEType
}

// fmtError implements Provider.
func (webhook *Webhook) fmtError(respBody io.Reader) error {
	body, err := io.ReadAll(respBody)
	if err != nil || len(body) == 0 {
		return ErrUnknownError
	}
	return rawError(body)
}

func (webhook *Webhook) MarshalMessage(logMsg *LogMessage) ([]byte, error) {
	title, err := sonic.Marshal(logMsg.Title)
	if err != nil {
		return nil, err
	}
	fields, err := logMsg.Body.Format(LogFormatRawJSON)
	if err != nil {
		return nil, err
	}
	var color string
	if webhook.ColorMode == "hex" {
		color = logMsg.Color.HexString()
	} else {
		color = logMsg.Color.DecString()
	}
	message, err := logMsg.Body.Format(LogFormatMarkdown)
	if err != nil {
		return nil, err
	}
	if webhook.MIMEType == MimeTypeJSON {
		message, err = sonic.Marshal(string(message))
		if err != nil {
			return nil, err
		}
	}
	plTempl := strings.NewReplacer(
		"$title", string(title),
		"$message", string(message),
		"$fields", string(fields),
		"$color", color,
	)
	var pl string
	if webhook.Template != "" {
		pl = webhookTemplates[webhook.Template]
	} else {
		pl = webhook.Payload
	}
	pl = plTempl.Replace(pl)
	return []byte(pl), nil
}

func validateJSONPayload(payload string) bool {
	replacer := strings.NewReplacer(
		"$title", `""`,
		"$message", `""`,
		"$fields", `""`,
		"$color", "",
	)
	payload = replacer.Replace(payload)
	return sonic.Valid([]byte(payload))
}
