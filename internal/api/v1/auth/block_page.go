package auth

import (
	"html/template"
	"net/http"

	_ "embed"
)

//go:embed block_page.html
var blockPageHTML string

var blockPageTemplate = template.Must(template.New("block_page").Parse(blockPageHTML))

func WriteBlockPage(w http.ResponseWriter, status int, error string, logoutURL string) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	blockPageTemplate.Execute(w, map[string]string{
		"StatusText": http.StatusText(status),
		"Error":      error,
		"LogoutURL":  logoutURL,
	})
}
