package gphttp

import "net/http"

func IsSuccess(status int) bool {
	return status >= http.StatusOK && status < http.StatusMultipleChoices
}

func IsStatusCodeValid(status int) bool {
	return http.StatusText(status) != ""
}
