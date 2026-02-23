package rules

import (
	"io"
	"net/http"
	"strings"

	httputils "github.com/yusing/goutils/http"
)

type templateString struct {
	string

	isTemplate bool
}

type keyValueTemplate struct {
	key  string
	tmpl templateString
}

func (tmpl *keyValueTemplate) Unpack() (string, templateString) {
	return tmpl.key, tmpl.tmpl
}

func (tmpl *templateString) ExpandVars(w *httputils.ResponseModifier, req *http.Request, dst io.Writer) (phase PhaseFlag, err error) {
	if !tmpl.isTemplate {
		_, err := asBytesBufferLike(dst).WriteString(tmpl.string)
		return PhaseNone, err
	}

	return ExpandVars(w, req, tmpl.string, dst)
}

func (tmpl *templateString) ExpandVarsToString(w *httputils.ResponseModifier, r *http.Request) (string, PhaseFlag, error) {
	if !tmpl.isTemplate {
		return tmpl.string, PhaseNone, nil
	}

	var buf strings.Builder
	phase, err := tmpl.ExpandVars(w, r, &buf)
	if err != nil {
		return "", PhaseNone, err
	}
	return buf.String(), phase, nil
}

func (tmpl *templateString) Len() int {
	return len(tmpl.string)
}
