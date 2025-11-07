package idlewatcher

import (
	_ "embed"
	"html/template"
	"net/http"

	idlewatcher "github.com/yusing/godoxy/internal/idlewatcher/types"
)

type templateData struct {
	Title   string
	Message string

	FavIconPath        string
	LoadingPageCSSPath string
	LoadingPageJSPath  string
	WakeEventsPath     string
}

//go:embed html/loading_page.html
var loadingPage []byte
var loadingPageTmpl = template.Must(template.New("loading_page").Parse(string(loadingPage)))

//go:embed html/style.css
var cssBytes []byte

//go:embed html/loading.js
var jsBytes []byte

func (w *Watcher) writeLoadingPage(rw http.ResponseWriter) error {
	msg := w.cfg.ContainerName() + " is starting..."

	data := new(templateData)
	data.Title = w.cfg.ContainerName()
	data.Message = msg
	data.FavIconPath = idlewatcher.FavIconPath
	data.LoadingPageCSSPath = idlewatcher.LoadingPageCSSPath
	data.LoadingPageJSPath = idlewatcher.LoadingPageJSPath
	data.WakeEventsPath = idlewatcher.WakeEventsPath

	rw.Header().Set("Content-Type", "text/html; charset=utf-8")
	rw.Header().Set("Cache-Control", "no-cache")
	rw.Header().Add("Cache-Control", "no-store")
	rw.Header().Add("Cache-Control", "must-revalidate")
	rw.Header().Add("Connection", "close")
	err := loadingPageTmpl.Execute(rw, data)
	return err
}
