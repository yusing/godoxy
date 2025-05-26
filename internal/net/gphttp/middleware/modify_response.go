package middleware

import (
	"net/http"
)

type modifyResponse struct {
	ModifyRequestOpts
	Tracer
}

var ModifyResponse = NewMiddleware[modifyResponse]()

// modifyResponse implements ResponseModifier.
func (mr *modifyResponse) modifyResponse(resp *http.Response) error {
	mr.AddTraceResponse("before modify response", resp)
	if !mr.needVarSubstitution {
		mr.modifyHeaders(resp.Request, resp.Header)
	} else {
		mr.modifyHeadersWithVarSubstitution(resp.Request, resp, resp.Header)
	}
	mr.AddTraceResponse("after modify response", resp)
	return nil
}
