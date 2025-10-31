package middleware

import (
	"bytes"
	"io"
	"net/http"
	"strconv"

	"github.com/PuerkitoBio/goquery"
	"github.com/rs/zerolog/log"
	httputils "github.com/yusing/goutils/http"
	ioutils "github.com/yusing/goutils/io"
	"github.com/yusing/goutils/synk"
	"golang.org/x/net/html"
)

type modifyHTML struct {
	Target  string // css selector
	HTML    string // html to inject
	Replace bool   // replace the target element with the new html instead of appending it
}

var ModifyHTML = NewMiddleware[modifyHTML]()

func (m *modifyHTML) before(_ http.ResponseWriter, req *http.Request) bool {
	req.Header.Set("Accept-Encoding", "identity")
	return true
}

func readerWithRelease(b []byte, release func([]byte)) io.ReadCloser {
	return ioutils.NewHookReadCloser(io.NopCloser(bytes.NewReader(b)), func() {
		release(b)
	})
}

type eofReader struct{}

func (eofReader) Read([]byte) (int, error) { return 0, io.EOF }
func (eofReader) Close() error             { return nil }

// modifyResponse implements ResponseModifier.
func (m *modifyHTML) modifyResponse(resp *http.Response) error {
	// including text/html and application/xhtml+xml
	if !httputils.GetContentType(resp.Header).IsHTML() {
		return nil
	}

	// Skip modification for streaming/chunked responses to avoid blocking reads
	// Unknown content length or any transfer encoding indicates streaming.
	// if resp.ContentLength < 0 || len(resp.TransferEncoding) > 0 {
	// 	log.Debug().Str("url", fullURL(resp.Request)).Strs("transfer-encoding", resp.TransferEncoding).Msg("skipping modification for streaming/chunked response")
	// 	return nil
	// }

	// NOTE: do not put it in the defer, it will be used as resp.Body
	content, release, err := httputils.ReadAllBody(resp)
	resp.Body.Close()
	if err != nil {
		log.Err(err).Str("url", fullURL(resp.Request)).Msg("failed to read response body")
		// Fail open: do not abort the response. Return an empty body safely.
		resp.ContentLength = 0
		resp.Header.Set("Content-Length", "0")
		resp.Header.Del("Transfer-Encoding")
		resp.Header.Del("Trailer")
		resp.Header.Del("Content-Encoding")
		resp.Body = eofReader{}
		return nil
	}

	doc, err := goquery.NewDocumentFromReader(bytes.NewReader(content))
	if err != nil {
		// invalid html, restore the original body
		resp.Body = readerWithRelease(content, release)
		log.Err(err).Str("url", fullURL(resp.Request)).Msg("invalid html found")
		return nil
	}

	ele := doc.Find(m.Target)
	if ele.Length() == 0 {
		// no target found, restore the original body
		resp.Body = readerWithRelease(content, release)
		return nil
	}

	if m.Replace {
		// replace all matching elements
		ele.ReplaceWithHtml(m.HTML)
	} else {
		// append to the first matching element
		ele.First().AppendHtml(m.HTML)
	}

	pool := synk.GetUnsizedBytesPool()

	buf := pool.GetBuffer()
	err = buildHTML(doc, buf)
	if err != nil {
		pool.PutBuffer(buf)
		log.Err(err).Str("url", fullURL(resp.Request)).Msg("failed to build html")
		// invalid html, restore the original body
		resp.Body = readerWithRelease(content, release)
		return err
	}

	release(content)
	resp.ContentLength = int64(buf.Len())
	resp.Header.Set("Content-Length", strconv.Itoa(buf.Len()))
	resp.Header.Del("Transfer-Encoding")
	resp.Header.Del("Trailer")
	resp.Header.Del("Content-Encoding")
	resp.Header.Set("Content-Type", "text/html; charset=utf-8")
	resp.Body = readerWithRelease(buf.Bytes(), func(_ []byte) {
		pool.PutBuffer(buf)
	})
	return nil
}

// copied and modified from	(*goquery.Selection).Html()
func buildHTML(s *goquery.Document, buf *bytes.Buffer) error {
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
			err := html.Render(buf, c)
			if err != nil {
				return err
			}
		}
	}
	return nil
}

func fullURL(req *http.Request) string {
	return req.Host + req.RequestURI
}
