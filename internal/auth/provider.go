package auth

import "net/http"

// LoginResult tells the caller whether LoginHandler completed the response or
// only refreshed credentials for the caller to handle at its HTTP boundary.
type LoginResult uint8

const (
	LoginResponseHandled LoginResult = iota
	LoginSessionRefreshed
)

type Provider interface {
	CheckToken(r *http.Request) error
	LoginHandler(w http.ResponseWriter, r *http.Request) LoginResult
	PostAuthCallbackHandler(w http.ResponseWriter, r *http.Request)
	LogoutHandler(w http.ResponseWriter, r *http.Request)
}
