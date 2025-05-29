package middleware

import (
	"net/http"
)

type modifyResponse struct {
	ModifyRequestOpts
}

var ModifyResponse = NewMiddleware[modifyResponse]()

// modifyResponse implements ResponseModifier.
func (mr *modifyResponse) modifyResponse(resp *http.Response) error {
	if !mr.needVarSubstitution {
		mr.modifyHeaders(resp.Request, resp.Header)
	} else {
		mr.modifyHeadersWithVarSubstitution(resp.Request, resp, resp.Header)
	}
	return nil
}
