package rules

import (
	"bytes"
	"io"
	"net/http"
)

type templateOrStr interface {
	Execute(w io.Writer, data any) error
}

type strTemplate string

func (t strTemplate) Execute(w io.Writer, _ any) error {
	n, err := w.Write([]byte(t))
	if err != nil {
		return err
	}
	if n != len(t) {
		return io.ErrShortWrite
	}
	return nil
}

type keyValueTemplate = Tuple[string, templateOrStr]

func executeRequestTemplateString(tmpl templateOrStr, r *http.Request) (string, error) {
	var buf bytes.Buffer
	err := tmpl.Execute(&buf, reqResponseTemplateData{Request: r})
	if err != nil {
		return "", err
	}
	return buf.String(), nil
}

func executeRequestTemplateTo(tmpl templateOrStr, o io.Writer, r *http.Request) error {
	return tmpl.Execute(o, reqResponseTemplateData{Request: r})
}

func executeReqRespTemplateTo(tmpl templateOrStr, o io.Writer, w http.ResponseWriter, r *http.Request) error {
	return tmpl.Execute(o, reqResponseTemplateData{Request: r, Response: GetInitResponseModifier(w).Response()})
}
