package gperr

import (
	"encoding/json"
	"errors"
	"fmt"
)

func newError(message string) error {
	return errStr(message)
}

func New(message string) Error {
	if message == "" {
		return nil
	}
	return &baseError{newError(message)}
}

func Errorf(format string, args ...any) Error {
	return &baseError{fmt.Errorf(format, args...)}
}

// Wrap wraps message in front of the error message.
func Wrap(err error, message ...string) Error {
	if err == nil {
		return nil
	}
	if len(message) == 0 || message[0] == "" {
		return wrap(err)
	}
	//nolint:errorlint
	switch err := err.(type) {
	case *baseError:
		err.Err = fmt.Errorf("%s: %w", message[0], err.Err)
		return err
	case *nestedError:
		err.Err = fmt.Errorf("%s: %w", message[0], err.Err)
		return err
	}
	return &baseError{fmt.Errorf("%s: %w", message[0], err)}
}

func wrap(err error) Error {
	if err == nil {
		return nil
	}
	//nolint:errorlint
	switch err := err.(type) {
	case Error:
		return err
	}
	return &baseError{err}
}

func IsJSONMarshallable(err error) bool {
	switch err := err.(type) {
	case *nestedError, *withSubject:
		return true
	case *baseError:
		return IsJSONMarshallable(err.Err)
	default:
		var v json.Marshaler
		return errors.As(err, &v)
	}
}

func Join(errors ...error) Error {
	n := 0
	for _, err := range errors {
		if err != nil {
			n++
		}
	}
	if n == 0 {
		return nil
	}
	errs := make([]error, n)
	i := 0
	for _, err := range errors {
		if err != nil {
			errs[i] = err
			i++
		}
	}
	return &nestedError{Extras: errs}
}

func Collect[T any, Err error, Arg any, Func func(Arg) (T, Err)](eb *Builder, fn Func, arg Arg) T {
	result, err := fn(arg)
	eb.Add(err)
	return result
}
