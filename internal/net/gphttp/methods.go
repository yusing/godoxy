package gphttp

import "net/http"

func IsMethodValid(method string) bool {
	switch method {
	case http.MethodGet,
		http.MethodHead,
		http.MethodPost,
		http.MethodPut,
		http.MethodPatch,
		http.MethodDelete,
		http.MethodConnect,
		http.MethodOptions,
		http.MethodTrace:
		return true
	default:
		return false
	}
}
