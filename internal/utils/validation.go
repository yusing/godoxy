package utils

import (
	"github.com/go-playground/validator/v10"
	"github.com/yusing/go-proxy/internal/gperr"
)

var validate = validator.New()

var ErrValidationError = gperr.New("validation error")

func Validate(v any) gperr.Error {
	err := validate.Struct(v)
	if err != nil {
		return ErrValidationError.With(err)
	}
	return nil
}

type CustomValidator interface {
	Validate() gperr.Error
}

func MustRegisterValidation(tag string, fn validator.Func) {
	err := validate.RegisterValidation(tag, fn)
	if err != nil {
		panic(err)
	}
}
