package auth

import "net/http"

type Provider interface {
	CheckToken(r *http.Request) error
}
