package middleware

import (
	"net/http"
	"strconv"

	gperr "github.com/yusing/goutils/errs"
)

type middlewareChain struct {
	befores    []RequestModifier
	respHeader []ResponseModifier
	respBody   []ResponseModifier
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
			if isBodyResponseModifier(mr) {
				chainMid.respBody = append(chainMid.respBody, mr)
			} else {
				chainMid.respHeader = append(chainMid.respHeader, mr)
			}
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
	for i, mr := range m.respHeader {
		if err := mr.modifyResponse(resp); err != nil {
			return gperr.PrependSubject(err, strconv.Itoa(i))
		}
	}
	if len(m.respBody) == 0 || !canBufferAndModifyResponseBody(responseHeaderForBodyRewriteGate(resp)) {
		return nil
	}
	headerLen := len(m.respHeader)
	for i, mr := range m.respBody {
		if err := mr.modifyResponse(resp); err != nil {
			return gperr.PrependSubject(err, strconv.Itoa(i+headerLen))
		}
	}
	return nil
}

func modifyResponseHeadersOnly(mr ResponseModifier, resp *http.Response) error {
	if chain, ok := mr.(*middlewareChain); ok {
		for i, mr := range chain.respHeader {
			if err := mr.modifyResponse(resp); err != nil {
				return gperr.PrependSubject(err, strconv.Itoa(i))
			}
		}
		return nil
	}
	if isBodyResponseModifier(mr) {
		return nil
	}
	return mr.modifyResponse(resp)
}

func modifyResponseBodyOnly(mr ResponseModifier, resp *http.Response) error {
	if chain, ok := mr.(*middlewareChain); ok {
		headerLen := len(chain.respHeader)
		for i, mr := range chain.respBody {
			if err := mr.modifyResponse(resp); err != nil {
				return gperr.PrependSubject(err, strconv.Itoa(i+headerLen))
			}
		}
		return nil
	}
	if !isBodyResponseModifier(mr) {
		return nil
	}
	return mr.modifyResponse(resp)
}

func isBodyResponseModifier(mr ResponseModifier) bool {
	if chain, ok := mr.(*middlewareChain); ok {
		return len(chain.respBody) > 0
	}
	if bypass, ok := mr.(*checkBypass); ok {
		return isBodyResponseModifier(bypass.modRes)
	}
	_, ok := mr.(BodyResponseModifier)
	return ok
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
