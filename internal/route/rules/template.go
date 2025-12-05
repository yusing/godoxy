package rules

import (
	"io"
	"net/http"
	"strings"
	"unsafe"

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

func (tmpl *templateString) ExpandVars(w http.ResponseWriter, req *http.Request, dstW io.Writer) error {
	if !tmpl.isTemplate {
		_, err := dstW.Write(strtobNoCopy(tmpl.string))
		return err
	}

	return ExpandVars(httputils.GetInitResponseModifier(w), req, tmpl.string, dstW)
}

func (tmpl *templateString) ExpandVarsToString(w http.ResponseWriter, req *http.Request) (string, error) {
	if !tmpl.isTemplate {
		return tmpl.string, nil
	}

	var buf strings.Builder
	err := ExpandVars(httputils.GetInitResponseModifier(w), req, tmpl.string, &buf)
	if err != nil {
		return "", err
	}
	return buf.String(), nil
}

func (tmpl *templateString) Len() int {
	return len(tmpl.string)
}

func strtobNoCopy(s string) []byte {
	return unsafe.Slice(unsafe.StringData(s), len(s))
}
