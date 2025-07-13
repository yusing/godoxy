package middleware

import (
	"bytes"
	"io"
	"net/http"
	"strconv"

	"github.com/PuerkitoBio/goquery"
	"github.com/rs/zerolog/log"
	gphttp "github.com/yusing/go-proxy/internal/net/gphttp"
	"golang.org/x/net/html"
)

type modifyHTML struct {
	Target  string // css selector
	HTML    string // html to inject
	Replace bool   // replace the target element with the new html instead of appending it
}

var ModifyHTML = NewMiddleware[modifyHTML]()

func (m *modifyHTML) before(w http.ResponseWriter, req *http.Request) bool {
	req.Header.Set("Accept-Encoding", "")
	return true
}

// modifyResponse implements ResponseModifier.
func (m *modifyHTML) modifyResponse(resp *http.Response) error {
	// including text/html and application/xhtml+xml
	if !gphttp.GetContentType(resp.Header).IsHTML() {
		return nil
	}

	content, err := io.ReadAll(resp.Body)
	if err != nil {
		resp.Body.Close()
		return err
	}
	resp.Body.Close()

	doc, err := goquery.NewDocumentFromReader(bytes.NewReader(content))
	if err != nil {
		// invalid html, restore the original body
		resp.Body = io.NopCloser(bytes.NewReader(content))
		log.Err(err).Str("url", fullURL(resp.Request)).Msg("invalid html found")
		return nil
	}

	ele := doc.Find(m.Target)
	if ele.Length() == 0 {
		// no target found, restore the original body
		resp.Body = io.NopCloser(bytes.NewReader(content))
		return nil
	}

	if m.Replace {
		// replace all matching elements
		ele.ReplaceWithHtml(m.HTML)
	} else {
		// append to the first matching element
		ele.First().AppendHtml(m.HTML)
	}

	h, err := buildHTML(doc)
	if err != nil {
		return err
	}
	resp.ContentLength = int64(len(h))
	resp.Header.Set("Content-Length", strconv.Itoa(len(h)))
	resp.Header.Set("Content-Type", "text/html; charset=utf-8")
	resp.Body = io.NopCloser(bytes.NewReader(h))
	return nil
}

// copied and modified from	(*goquery.Selection).Html()
func buildHTML(s *goquery.Document) (ret []byte, err error) {
	var buf bytes.Buffer

	// Merge all head nodes into one
	headNodes := s.Find("head")
	if headNodes.Length() > 1 {
		// Get the first head node to merge everything into
		firstHead := headNodes.First()

		// Merge content from all other head nodes into the first one
		headNodes.Slice(1, headNodes.Length()).Each(func(i int, otherHead *goquery.Selection) {
			// Move all children from other head nodes to the first head
			otherHead.Children().Each(func(j int, child *goquery.Selection) {
				firstHead.AppendSelection(child)
			})
		})

		// Remove the duplicate head nodes (keep only the first one)
		headNodes.Slice(1, headNodes.Length()).Remove()
	}

	if len(s.Nodes) > 0 {
		for c := s.Nodes[0].FirstChild; c != nil; c = c.NextSibling {
			err = html.Render(&buf, c)
			if err != nil {
				return
			}
		}
		ret = buf.Bytes()
	}
	return
}

func fullURL(req *http.Request) string {
	return req.Host + req.RequestURI
}
