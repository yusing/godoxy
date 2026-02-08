package auth

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	expect "github.com/yusing/goutils/testing"
	"golang.org/x/crypto/bcrypt"
)

func newMockUserPassAuth() *UserPassAuth {
	return &UserPassAuth{
		username: "username",
		pwdHash:  expect.Must(bcrypt.GenerateFromPassword([]byte("password"), bcrypt.DefaultCost)),
		secret:   []byte("abcdefghijklmnopqrstuvwxyz"),
		tokenTTL: time.Hour,
	}
}

func TestUserPassValidateCredentials(t *testing.T) {
	auth := newMockUserPassAuth()
	err := auth.validatePassword("username", "password")
	expect.NoError(t, err)
	err = auth.validatePassword("username", "wrong-password")
	expect.ErrorIs(t, bcrypt.ErrMismatchedHashAndPassword, err)
	err = auth.validatePassword("wrong-username", "password")
	expect.ErrorIs(t, ErrInvalidUsername, err)
}

func TestUserPassCheckToken(t *testing.T) {
	auth := newMockUserPassAuth()
	token, err := auth.NewToken()
	expect.NoError(t, err)
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
			expect.True(t, err != nil)
		} else {
			expect.NoError(t, err)
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
			Body: io.NopCloser(bytes.NewReader(expect.Must(json.Marshal(tt.creds)))),
		}
		auth.PostAuthCallbackHandler(w, req)
		if tt.wantErr {
			expect.Equal(t, w.Code, http.StatusBadRequest)
		} else {
			setCookie := expect.Must(http.ParseSetCookie(w.Header().Get("Set-Cookie")))
			expect.True(t, setCookie.Name == auth.TokenCookieName())
			expect.True(t, setCookie.Value != "")
			expect.Equal(t, setCookie.Domain, "example.com")
			expect.Equal(t, setCookie.Path, "/")
			expect.Equal(t, setCookie.SameSite, http.SameSiteLaxMode)
			expect.Equal(t, setCookie.HttpOnly, true)
			expect.Equal(t, w.Code, http.StatusOK)
		}
	}
}
