package middleware

import (
	"net/http"

	"github.com/yusing/go-proxy/internal/common"
	"github.com/yusing/go-proxy/internal/gperr"
)

type middlewareChain struct {
	befores  []RequestModifier
	modResps []ResponseModifier
}

// TODO: check conflict or duplicates.
func NewMiddlewareChain(name string, chain []*Middleware) *Middleware {
	chainMid := &middlewareChain{befores: []RequestModifier{}, modResps: []ResponseModifier{}}
	m := &Middleware{name: name, impl: chainMid}

	for _, comp := range chain {
		if before, ok := comp.impl.(RequestModifier); ok {
			chainMid.befores = append(chainMid.befores, before)
		}
		if mr, ok := comp.impl.(ResponseModifier); ok {
			chainMid.modResps = append(chainMid.modResps, mr)
		}
		comp.setParent(m)
	}

	if common.IsDebug {
		for _, child := range chain {
			child.enableTrace()
		}
		m.enableTrace()
	}
	return m
}

// before implements RequestModifier.
func (m *middlewareChain) before(w http.ResponseWriter, r *http.Request) (proceedNext bool) {
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
	errs := gperr.NewBuilder("modify response errors")
	for i, mr := range m.modResps {
		if err := mr.modifyResponse(resp); err != nil {
			errs.Add(gperr.Wrap(err).Subjectf("%d", i))
		}
	}
	return errs.Error()
}
