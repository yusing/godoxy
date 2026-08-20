package autocert

import (
	"crypto"

	"github.com/go-acme/lego/v5/acme"
)

type User struct {
	Email        string
	Registration *acme.ExtendedAccount
	Key          crypto.Signer
}

func (u *User) GetEmail() string {
	return u.Email
}

func (u *User) GetRegistration() *acme.ExtendedAccount {
	return u.Registration
}

func (u *User) GetPrivateKey() crypto.Signer {
	return u.Key
}
