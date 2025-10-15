package serialization

import (
	"reflect"

	"github.com/go-playground/validator/v10"
	gperr "github.com/yusing/goutils/errs"
)

var validate = validator.New()

var ErrValidationError = gperr.New("validation error")

func Validator() *validator.Validate {
	return validate
}

func MustRegisterValidation(tag string, fn validator.Func) {
	err := validate.RegisterValidation(tag, fn)
	if err != nil {
		panic(err)
	}
}

type CustomValidator interface {
	Validate() gperr.Error
}

var validatorType = reflect.TypeFor[CustomValidator]()

func ValidateWithCustomValidator(v reflect.Value) gperr.Error {
	if v.Kind() == reflect.Pointer {
		if v.IsNil() {
			// return nil
			return validateWithValidator(reflect.New(v.Type().Elem()))
		}
		if v.Type().Implements(validatorType) {
			return v.Interface().(CustomValidator).Validate()
		}
		return validateWithValidator(v.Elem())
	} else {
		vt := v.Type()
		if vt.PkgPath() != "" { // not a builtin type
			if vt.Implements(validatorType) {
				return v.Interface().(CustomValidator).Validate()
			}
			if v.CanAddr() {
				return validateWithValidator(v.Addr())
			}
		}
	}
	return nil
}

func validateWithValidator(v reflect.Value) gperr.Error {
	if v.Type().Implements(validatorType) {
		return v.Interface().(CustomValidator).Validate()
	}
	return nil
}
