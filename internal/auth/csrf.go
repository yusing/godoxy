package auth

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"strings"

	"github.com/yusing/godoxy/internal/common"
	"golang.org/x/crypto/hkdf"
)

const (
	CSRFCookieName  = "godoxy_csrf"
	CSRFHKDFSalt    = "godoxy-csrf"
	CSRFHeaderName  = "X-CSRF-Token"
	csrfTokenLength = 32
)

// csrfSecret is derived from API_JWT_SECRET via HKDF for cryptographic
// separation from JWT signing. Falls back to an ephemeral random key
// for OIDC-only setups where no JWT secret is configured.
var csrfSecret = func() []byte {
	if common.APIJWTSecret != nil {
		return hkdf.Extract(sha256.New, common.APIJWTSecret, []byte(CSRFHKDFSalt))
	}
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		panic("failed to generate CSRF secret: " + err.Error())
	}
	return b
}()

func GenerateCSRFToken() (string, error) {
	nonce := make([]byte, csrfTokenLength)
	if _, err := rand.Read(nonce); err != nil {
		return "", err
	}
	nonceHex := hex.EncodeToString(nonce)
	return nonceHex + "." + csrfSign(nonceHex), nil
}

// ValidateCSRFToken checks the HMAC signature embedded in the token.
// This prevents subdomain cookie-injection attacks where an attacker
// sets a forged CSRF cookie — they cannot produce a valid signature
// without the ephemeral secret.
func ValidateCSRFToken(token string) bool {
	nonce, sig, ok := strings.Cut(token, ".")
	if !ok || len(nonce) != csrfTokenLength*2 {
		return false
	}
	return hmac.Equal([]byte(sig), []byte(csrfSign(nonce)))
}

func csrfSign(nonce string) string {
	mac := hmac.New(sha256.New, csrfSecret)
	mac.Write([]byte(nonce))
	return hex.EncodeToString(mac.Sum(nil))
}

func SetCSRFCookie(w http.ResponseWriter, r *http.Request, token string) {
	http.SetCookie(w, &http.Cookie{
		Name:     CSRFCookieName,
		Value:    token,
		HttpOnly: false,
		Secure:   common.APIJWTSecure,
		SameSite: http.SameSiteStrictMode,
		Path:     "/",
	})
}

func ClearCSRFCookie(w http.ResponseWriter, r *http.Request) {
	http.SetCookie(w, &http.Cookie{
		Name:     CSRFCookieName,
		Value:    "",
		MaxAge:   -1,
		HttpOnly: false,
		Secure:   common.APIJWTSecure,
		SameSite: http.SameSiteStrictMode,
		Path:     "/",
	})
}
