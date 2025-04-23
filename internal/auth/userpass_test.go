package auth

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	. "github.com/yusing/go-proxy/internal/utils/testing"
	"golang.org/x/crypto/bcrypt"
)

func newMockUserPassAuth() *UserPassAuth {
	return &UserPassAuth{
		username: "username",
		pwdHash:  Must(bcrypt.GenerateFromPassword([]byte("password"), bcrypt.DefaultCost)),
		secret:   []byte("abcdefghijklmnopqrstuvwxyz"),
		tokenTTL: time.Hour,
	}
}

func TestUserPassValidateCredentials(t *testing.T) {
	auth := newMockUserPassAuth()
	err := auth.validatePassword("username", "password")
	ExpectNoError(t, err)
	err = auth.validatePassword("username", "wrong-password")
	ExpectError(t, ErrInvalidPassword, err)
	err = auth.validatePassword("wrong-username", "password")
	ExpectError(t, ErrInvalidUsername, err)
}

func TestUserPassCheckToken(t *testing.T) {
	auth := newMockUserPassAuth()
	token, err := auth.NewToken()
	ExpectNoError(t, err)
	tests := []struct {
		token   string
		wantErr bool
	}{
		{
			token:   token,
			wantErr: false,
		},
		{
			token:   "invalid-token",
			wantErr: true,
		},
		{
			token:   "",
			wantErr: true,
		},
	}
	for _, tt := range tests {
		req := &http.Request{Header: http.Header{}}
		if tt.token != "" {
			req.Header.Set("Cookie", auth.TokenCookieName()+"="+tt.token)
		}
		err = auth.CheckToken(req)
		if tt.wantErr {
			ExpectTrue(t, err != nil)
		} else {
			ExpectNoError(t, err)
		}
	}
}

func TestUserPassLoginCallbackHandler(t *testing.T) {
	type cred struct {
		User string `json:"username"`
		Pass string `json:"password"`
	}
	auth := newMockUserPassAuth()
	tests := []struct {
		creds   cred
		wantErr bool
	}{
		{
			creds: cred{
				User: "username",
				Pass: "password",
			},
			wantErr: false,
		},
		{
			creds: cred{
				User: "username",
				Pass: "wrong-password",
			},
			wantErr: true,
		},
	}
	for _, tt := range tests {
		w := httptest.NewRecorder()
		req := &http.Request{
			Host: "app.example.com",
			Body: io.NopCloser(bytes.NewReader(Must(json.Marshal(tt.creds)))),
		}
		auth.LoginCallbackHandler(w, req)
		if tt.wantErr {
			ExpectEqual(t, w.Code, http.StatusUnauthorized)
		} else {
			setCookie := Must(http.ParseSetCookie(w.Header().Get("Set-Cookie")))
			ExpectTrue(t, setCookie.Name == auth.TokenCookieName())
			ExpectTrue(t, setCookie.Value != "")
			ExpectEqual(t, setCookie.Domain, "example.com")
			ExpectEqual(t, setCookie.Path, "/")
			ExpectEqual(t, setCookie.SameSite, http.SameSiteLaxMode)
			ExpectEqual(t, setCookie.HttpOnly, true)
			ExpectEqual(t, w.Code, http.StatusOK)
		}
	}
}
