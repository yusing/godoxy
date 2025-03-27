package middleware

import (
	"net/http"
	"path/filepath"
	"strings"
)

type (
	modifyRequest struct {
		ModifyRequestOpts
		Tracer
	}
	// order: add_prefix -> set_headers -> add_headers -> hide_headers
	ModifyRequestOpts struct {
		SetHeaders  map[string]string
		AddHeaders  map[string]string
		HideHeaders []string
		AddPrefix   string

		needVarSubstitution bool
	}
)

var ModifyRequest = NewMiddleware[modifyRequest]()

// finalize implements MiddlewareFinalizer.
func (mr *ModifyRequestOpts) finalize() {
	mr.checkVarSubstitution()
}

// before implements RequestModifier.
func (mr *modifyRequest) before(w http.ResponseWriter, r *http.Request) (proceed bool) {
	mr.AddTraceRequest("before modify request", r)

	mr.addPrefix(r, nil, r.URL.Path)
	mr.modifyHeaders(r, nil, r.Header)
	mr.AddTraceRequest("after modify request", r)
	return true
}

func (mr *ModifyRequestOpts) checkVarSubstitution() {
	for _, m := range []map[string]string{mr.SetHeaders, mr.AddHeaders} {
		for _, v := range m {
			if strings.ContainsRune(v, '$') {
				mr.needVarSubstitution = true
				return
			}
		}
	}
}

func (mr *ModifyRequestOpts) modifyHeaders(req *http.Request, resp *http.Response, headers http.Header) {
	if !mr.needVarSubstitution {
		for k, v := range mr.SetHeaders {
			if req != nil && strings.EqualFold(k, "host") {
				defer func() {
					req.Host = v
				}()
			}
			headers[k] = []string{v}
		}
		for k, v := range mr.AddHeaders {
			headers[k] = append(headers[k], v)
		}
	} else {
		for k, v := range mr.SetHeaders {
			if req != nil && strings.EqualFold(k, "host") {
				defer func() {
					req.Host = varReplace(req, resp, v)
				}()
			}
			headers[k] = []string{varReplace(req, resp, v)}
		}
		for k, v := range mr.AddHeaders {
			headers[k] = append(headers[k], varReplace(req, resp, v))
		}
	}

	for _, k := range mr.HideHeaders {
		delete(headers, k)
	}
}

func (mr *modifyRequest) addPrefix(r *http.Request, _ *http.Response, path string) {
	if len(mr.AddPrefix) == 0 {
		return
	}

	r.URL.Path = filepath.Join(mr.AddPrefix, path)
}
