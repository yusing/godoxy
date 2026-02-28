package middleware

import (
	"fmt"
	"net/http"
	"strconv"

	gperr "github.com/yusing/goutils/errs"
)

type middlewareChain struct {
	befores  []RequestModifier
	modResps []ResponseModifier
}

// TODO: check conflict or duplicates.
func NewMiddlewareChain(name string, chain []*Middleware) *Middleware {
	chainMid := &middlewareChain{}
	m := &Middleware{name: name, impl: chainMid}

	for _, comp := range chain {
		if before, ok := comp.impl.(RequestModifier); ok {
			chainMid.befores = append(chainMid.befores, before)
		}
		if mr, ok := comp.impl.(ResponseModifier); ok {
			chainMid.modResps = append(chainMid.modResps, mr)
		}
	}
	return m
}

// before implements RequestModifier.
func (m *middlewareChain) before(w http.ResponseWriter, r *http.Request) (proceedNext bool) {
	if len(m.befores) == 0 {
		return true
	}
	for _, b := range m.befores {
		if proceedNext = b.before(w, r); !proceedNext {
			return false
		}
	}
	return true
}

// modifyResponse implements ResponseModifier.
func (m *middlewareChain) modifyResponse(resp *http.Response) error {
	if len(m.modResps) == 0 {
		return nil
	}
	for i, mr := range m.modResps {
		if err := modifyResponseWithBodyRewriteGate(mr, resp); err != nil {
			return gperr.PrependSubject(err, strconv.Itoa(i))
		}
	}
	return nil
}

func modifyResponseWithBodyRewriteGate(mr ResponseModifier, resp *http.Response) error {
	originalBody := resp.Body
	originalContentLength := resp.ContentLength
	allowBodyRewrite := canBufferAndModifyResponseBody(responseHeaderForBodyRewriteGate(resp))

	if err := mr.modifyResponse(resp); err != nil {
		return err
	}

	if allowBodyRewrite || resp.Body == originalBody {
		return nil
	}

	if resp.Body != nil {
		if err := resp.Body.Close(); err != nil {
			return fmt.Errorf("close rewritten body: %w", err)
		}
	}
	if originalBody == nil || originalBody == http.NoBody {
		resp.Body = http.NoBody
	} else {
		resp.Body = originalBody
	}
	resp.ContentLength = originalContentLength
	if originalContentLength >= 0 {
		resp.Header.Set("Content-Length", strconv.FormatInt(originalContentLength, 10))
	} else {
		resp.Header.Del("Content-Length")
	}
	return nil
}

func responseHeaderForBodyRewriteGate(resp *http.Response) http.Header {
	h := resp.Header.Clone()
	if len(resp.TransferEncoding) > 0 && len(h.Values("Transfer-Encoding")) == 0 {
		h["Transfer-Encoding"] = append([]string(nil), resp.TransferEncoding...)
	}
	if resp.ContentLength >= 0 && h.Get("Content-Length") == "" {
		h.Set("Content-Length", strconv.FormatInt(resp.ContentLength, 10))
	}
	return h
}
