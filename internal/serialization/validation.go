package serialization

import (
	"reflect"

	"github.com/go-playground/validator/v10"
	"github.com/puzpuzpuz/xsync/v4"
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

var newValueMap = xsync.NewMap[reflect.Type, reflect.Value](xsync.WithPresize(100))

func ValidateWithCustomValidator(v reflect.Value) gperr.Error {
	vt := v.Type()
	if v.Kind() == reflect.Pointer {
		elemType := vt.Elem()
		if vt.Implements(validatorType) {
			if v.IsNil() {
				newValue, _ := newValueMap.LoadOrCompute(elemType, func() (reflect.Value, bool) {
					return reflect.New(elemType), false
				})
				return newValue.Interface().(CustomValidator).Validate()
			}
			return v.Interface().(CustomValidator).Validate()
		}
		if elemType.Implements(validatorType) {
			return v.Elem().Interface().(CustomValidator).Validate()
		}
	} else {
		if vt.PkgPath() != "" { // not a builtin type
			// prioritize pointer method
			if v.CanAddr() {
				vAddr := v.Addr()
				if vAddr.Type().Implements(validatorType) {
					return vAddr.Interface().(CustomValidator).Validate()
				}
			}
			// fallback to value method
			if vt.Implements(validatorType) {
				return v.Interface().(CustomValidator).Validate()
			}
		}
	}
	return nil
}
