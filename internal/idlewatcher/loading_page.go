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

//go:embed html/loading_page.min.html
var loadingPage []byte
var loadingPageTmpl = template.Must(template.New("loading_page").Parse(string(loadingPage)))

//go:embed html/style.css
var cssBytes []byte

//go:embed html/loading.min.js
var jsBytes []byte

func setNoStoreHeaders(header http.Header) {
	header.Set("Cache-Control", "no-store, no-cache, must-revalidate, max-age=0")
	header.Set("Pragma", "no-cache")
	header.Set("Expires", "0")
}

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
	setNoStoreHeaders(rw.Header())
	rw.Header().Add("Connection", "close")
	err := loadingPageTmpl.Execute(rw, data)
	return err
}
