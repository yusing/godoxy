package accesslog

import (
	"iter"
	"net/http"
	"net/url"

	"github.com/rs/zerolog"
)

type (
	FieldConfig struct {
		Default FieldMode            `json:"default" validate:"omitempty,oneof=keep drop redact"`
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

type mapStringStringIter interface {
	Iter(yield func(k string, v []string) bool)
	MarshalZerologObject(e *zerolog.Event)
}

type mapStringStringSlice struct {
	m map[string][]string
}

func (m mapStringStringSlice) Iter(yield func(k string, v []string) bool) {
	for k, v := range m.m {
		if !yield(k, v) {
			return
		}
	}
}

func (m mapStringStringSlice) MarshalZerologObject(e *zerolog.Event) {
	for k, v := range m.m {
		e.Strs(k, v)
	}
}

type mapStringStringRedacted struct {
	m map[string][]string
}

func (m mapStringStringRedacted) Iter(yield func(k string, v []string) bool) {
	for k := range m.m {
		if !yield(k, []string{RedactedValue}) {
			return
		}
	}
}

func (m mapStringStringRedacted) MarshalZerologObject(e *zerolog.Event) {
	for k, v := range m.Iter {
		e.Strs(k, v)
	}
}

type mapStringStringSliceWithConfig struct {
	m   map[string][]string
	cfg *FieldConfig
}

func (m mapStringStringSliceWithConfig) Iter(yield func(k string, v []string) bool) {
	var mode FieldMode
	var ok bool
	for k, v := range m.m {
		if mode, ok = m.cfg.Config[k]; !ok {
			mode = m.cfg.Default
		}
		switch mode {
		case FieldModeKeep:
			if !yield(k, v) {
				return
			}
		case FieldModeRedact:
			if !yield(k, []string{RedactedValue}) {
				return
			}
		}
	}
}

func (m mapStringStringSliceWithConfig) MarshalZerologObject(e *zerolog.Event) {
	for k, v := range m.Iter {
		e.Strs(k, v)
	}
}

type mapStringStringDrop struct{}

func (m mapStringStringDrop) Iter(yield func(k string, v []string) bool) {}
func (m mapStringStringDrop) MarshalZerologObject(e *zerolog.Event)      {}

var mapStringStringDropIter mapStringStringIter = mapStringStringDrop{}

func mapIter[Map http.Header | url.Values](cfg *FieldConfig, m Map) mapStringStringIter {
	if len(cfg.Config) == 0 {
		switch cfg.Default {
		case FieldModeKeep:
			return mapStringStringSlice{m: m}
		case FieldModeDrop:
			return mapStringStringDropIter
		case FieldModeRedact:
			return mapStringStringRedacted{m: m}
		}
	}
	return mapStringStringSliceWithConfig{m: m, cfg: cfg}
}

type slice[V any] struct {
	s      []V
	getKey func(V) string
	getVal func(V) string
	cfg    *FieldConfig
}

type sliceIter interface {
	Iter(yield func(k string, v string) bool)
	MarshalZerologObject(e *zerolog.Event)
}

func (s *slice[V]) Iter(yield func(k string, v string) bool) {
	for _, v := range s.s {
		k := s.getKey(v)
		var mode FieldMode
		var ok bool
		if mode, ok = s.cfg.Config[k]; !ok {
			mode = s.cfg.Default
		}
		switch mode {
		case FieldModeKeep:
			if !yield(k, s.getVal(v)) {
				return
			}
		case FieldModeRedact:
			if !yield(k, RedactedValue) {
				return
			}
		}
	}
}

type sliceDrop struct{}

func (s sliceDrop) Iter(yield func(k string, v string) bool) {}
func (s sliceDrop) MarshalZerologObject(e *zerolog.Event)    {}

var sliceDropIter sliceIter = sliceDrop{}

func (s *slice[V]) MarshalZerologObject(e *zerolog.Event) {
	for k, v := range s.Iter {
		e.Str(k, v)
	}
}

func iterSlice[V any](cfg *FieldConfig, s []V, getKey func(V) string, getVal func(V) string) sliceIter {
	if len(s) == 0 ||
		len(cfg.Config) == 0 && cfg.Default == FieldModeDrop {
		return sliceDropIter
	}
	return &slice[V]{s: s, getKey: getKey, getVal: getVal, cfg: cfg}
}

func (cfg *FieldConfig) IterHeaders(headers http.Header) iter.Seq2[string, []string] {
	return mapIter(cfg, headers).Iter
}

func (cfg *FieldConfig) ZerologHeaders(headers http.Header) zerolog.LogObjectMarshaler {
	return mapIter(cfg, headers)
}

func (cfg *FieldConfig) IterQuery(q url.Values) iter.Seq2[string, []string] {
	return mapIter(cfg, q).Iter
}

func (cfg *FieldConfig) ZerologQuery(q url.Values) zerolog.LogObjectMarshaler {
	return mapIter(cfg, q)
}

func cookieGetKey(c *http.Cookie) string {
	return c.Name
}

func cookieGetValue(c *http.Cookie) string {
	return c.Value
}

func (cfg *FieldConfig) IterCookies(cookies []*http.Cookie) iter.Seq2[string, string] {
	return iterSlice(cfg, cookies, cookieGetKey, cookieGetValue).Iter
}

func (cfg *FieldConfig) ZerologCookies(cookies []*http.Cookie) zerolog.LogObjectMarshaler {
	return iterSlice(cfg, cookies, cookieGetKey, cookieGetValue)
}
