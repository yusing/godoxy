package auth

import (
	"net/http"
)

type Provider interface {
	TokenCookieName() string
	CheckToken(r *http.Request) error
}
