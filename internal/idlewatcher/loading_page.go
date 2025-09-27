package idlewatcher

import (
	"bytes"
	_ "embed"
	"text/template"

	"github.com/yusing/goutils/http/httpheaders"
)

type templateData struct {
	CheckRedirectHeader string
	Title               string
	Message             string
}

//go:embed html/loading_page.html
var loadingPage []byte
var loadingPageTmpl = template.Must(template.New("loading_page").Parse(string(loadingPage)))

func (w *Watcher) makeLoadingPageBody() []byte {
	msg := w.cfg.ContainerName() + " is starting..."

	data := new(templateData)
	data.CheckRedirectHeader = httpheaders.HeaderGoDoxyCheckRedirect
	data.Title = w.cfg.ContainerName()
	data.Message = msg

	buf := bytes.NewBuffer(make([]byte, len(loadingPage)+len(data.Title)+len(data.Message)+len(httpheaders.HeaderGoDoxyCheckRedirect)))
	err := loadingPageTmpl.Execute(buf, data)
	if err != nil { // should never happen in production
		panic(err)
	}
	return buf.Bytes()
}
