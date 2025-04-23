package auth

import "net/http"

type Provider interface {
	CheckToken(r *http.Request) error
	LoginHandler(w http.ResponseWriter, r *http.Request)
	LogoutHandler(w http.ResponseWriter, r *http.Request)
}
