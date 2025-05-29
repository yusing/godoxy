package middleware

import (
	"net/http"
	"path/filepath"
	"strings"
)

type (
	modifyRequest struct {
		ModifyRequestOpts
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
	if len(mr.AddPrefix) != 0 {
		mr.addPrefix(r, r.URL.Path)
	}
	if !mr.needVarSubstitution {
		mr.modifyHeaders(r, r.Header)
	} else {
		mr.modifyHeadersWithVarSubstitution(r, nil, r.Header)
	}
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

func (mr *ModifyRequestOpts) modifyHeaders(req *http.Request, headers http.Header) {
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
	for _, k := range mr.HideHeaders {
		delete(headers, k)
	}
}

func (mr *ModifyRequestOpts) modifyHeadersWithVarSubstitution(req *http.Request, resp *http.Response, headers http.Header) {
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
	for _, k := range mr.HideHeaders {
		delete(headers, k)
	}
}

func (mr *modifyRequest) addPrefix(r *http.Request, path string) {
	r.URL.Path = filepath.Join(mr.AddPrefix, path)
}
