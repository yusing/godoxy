package middleware

import (
	"bytes"
	"errors"
	"fmt"
	"net/http"
	"os"
	"strings"
	"text/template"

	_ "embed"

	gperr "github.com/yusing/goutils/errs"
)

type themed struct {
	FontURL    string `json:"font_url"`
	FontFamily string `json:"font_family"`
	Theme      Theme  `json:"theme"` // predefined themes
	CSS        string `json:"css"`   // css url or content

	m modifyHTML
}

var Themed = NewMiddleware[themed]()

type Theme string

const (
	DarkTheme          Theme = "dark"
	DarkGreyTheme      Theme = "dark-grey"
	SolarizedDarkTheme Theme = "solarized-dark"
)

var (
	//go:embed themes/dark.css
	darkModeCSS string
	//go:embed themes/dark-grey.css
	darkGreyModeCSS string
	//go:embed themes/solarized-dark.css
	solarizedDarkModeCSS string
	//go:embed themes/font.css
	fontCSS string
)

var fontCSSTemplate = template.Must(template.New("fontCSS").Parse(fontCSS))

func (m *themed) before(w http.ResponseWriter, req *http.Request) bool {
	return m.m.before(w, req)
}

func (m *themed) modifyResponse(resp *http.Response) error {
	return m.m.modifyResponse(resp)
}

func (m *themed) finalize() error {
	m.m.Target = "body"
	if m.FontURL != "" && m.FontFamily != "" {
		buf := bytes.NewBuffer(nil)
		buf.WriteString(`<style type="text/css">`)
		err := fontCSSTemplate.Execute(buf, m)
		if err != nil {
			return err
		}
		buf.WriteString("</style>")
		m.m.HTML += buf.String()
	}
	if m.CSS != "" && m.Theme != "" {
		return errors.New("css and theme are mutually exclusive")
	}
	// credit: https://hackcss.egoist.dev
	if m.Theme != "" {
		switch m.Theme {
		case DarkTheme:
			m.m.HTML += wrapStyleTag(darkModeCSS)
		case DarkGreyTheme:
			m.m.HTML += wrapStyleTag(darkGreyModeCSS)
		case SolarizedDarkTheme:
			m.m.HTML += wrapStyleTag(solarizedDarkModeCSS)
		default:
			return gperr.PrependSubject(errors.New("invalid theme"), m.Theme)
		}
	}
	if m.CSS != "" {
		switch {
		case strings.HasPrefix(m.CSS, "https://"),
			strings.HasPrefix(m.CSS, "http://"),
			strings.HasPrefix(m.CSS, "/"):
			m.m.HTML += wrapStylesheetLinkTag(m.CSS)
		case strings.HasPrefix(m.CSS, "file://"):
			css, err := os.ReadFile(strings.TrimPrefix(m.CSS, "file://"))
			if err != nil {
				return err
			}
			m.m.HTML += wrapStyleTag(string(css))
		default:
			// inline css
			m.m.HTML += wrapStyleTag(m.CSS)
		}
	}
	return nil
}

func wrapStyleTag(css string) string {
	return fmt.Sprintf(`<style type="text/css">%s</style>`, css)
}

func wrapStylesheetLinkTag(href string) string {
	return fmt.Sprintf(`<link rel="stylesheet" href=%q>`, href)
}
