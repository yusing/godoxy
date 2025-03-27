package accesslog

import (
	"net/http"
	"net/url"
)

type (
	FieldConfig struct {
		Default FieldMode            `json:"default" validate:"oneof=keep drop redact"`
		Config  map[string]FieldMode `json:"config" validate:"dive,oneof=keep drop redact"`
	}
	FieldMode string
)

const (
	FieldModeKeep   FieldMode = "keep"
	FieldModeDrop   FieldMode = "drop"
	FieldModeRedact FieldMode = "redact"

	RedactedValue = "REDACTED"
)

func processMap[V any](cfg *FieldConfig, m map[string]V, redactedV V) map[string]V {
	if len(cfg.Config) == 0 {
		switch cfg.Default {
		case FieldModeKeep:
			return m
		case FieldModeDrop:
			return nil
		case FieldModeRedact:
			redacted := make(map[string]V)
			for k := range m {
				redacted[k] = redactedV
			}
			return redacted
		}
	}

	if len(m) == 0 {
		return m
	}

	newMap := make(map[string]V, len(m))
	for k := range m {
		var mode FieldMode
		var ok bool
		if mode, ok = cfg.Config[k]; !ok {
			mode = cfg.Default
		}
		switch mode {
		case FieldModeKeep:
			newMap[k] = m[k]
		case FieldModeRedact:
			newMap[k] = redactedV
		}
	}
	return newMap
}

func processSlice[V any, VReturn any](cfg *FieldConfig, s []V, getKey func(V) string, convert func(V) VReturn, redact func(V) VReturn) map[string]VReturn {
	if len(s) == 0 ||
		len(cfg.Config) == 0 && cfg.Default == FieldModeDrop {
		return nil
	}
	newMap := make(map[string]VReturn, len(s))
	for _, v := range s {
		var mode FieldMode
		var ok bool
		k := getKey(v)
		if mode, ok = cfg.Config[k]; !ok {
			mode = cfg.Default
		}
		switch mode {
		case FieldModeKeep:
			newMap[k] = convert(v)
		case FieldModeRedact:
			newMap[k] = redact(v)
		}
	}
	return newMap
}

func (cfg *FieldConfig) ProcessHeaders(headers http.Header) http.Header {
	return processMap(cfg, headers, []string{RedactedValue})
}

func (cfg *FieldConfig) ProcessQuery(q url.Values) url.Values {
	return processMap(cfg, q, []string{RedactedValue})
}

func (cfg *FieldConfig) ProcessCookies(cookies []*http.Cookie) map[string]string {
	return processSlice(cfg, cookies,
		func(c *http.Cookie) string {
			return c.Name
		},
		func(c *http.Cookie) string {
			return c.Value
		},
		func(c *http.Cookie) string {
			return RedactedValue
		})
}
