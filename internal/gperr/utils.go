package gperr

import (
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
		err.Err = &wrappedError{err.Err, message[0]}
		return err
	case *nestedError:
		err.Err = &wrappedError{err.Err, message[0]}
		return err
	}
	return &baseError{&wrappedError{err, message[0]}}
}

func Unwrap(err error) Error {
	//nolint:errorlint
	switch err := err.(type) {
	case interface{ Unwrap() []error }:
		return &nestedError{Extras: err.Unwrap()}
	case interface{ Unwrap() error }:
		return &baseError{err.Unwrap()}
	default:
		return &baseError{err}
	}
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

func JoinLines(main error, errors ...string) Error {
	errs := make([]error, len(errors))
	for i, err := range errors {
		if err == "" {
			continue
		}
		errs[i] = newError(err)
	}
	return &nestedError{Err: main, Extras: errs}
}

func Collect[T any, Err error, Arg any, Func func(Arg) (T, Err)](eb *Builder, fn Func, arg Arg) T {
	result, err := fn(arg)
	eb.Add(err)
	return result
}

func Normal(err error) []byte {
	if err == nil {
		return nil
	}
	return []byte(err.Error())
}

func Plain(err error) []byte {
	if err == nil {
		return nil
	}
	if p, ok := err.(PlainError); ok {
		return p.Plain()
	}
	return []byte(err.Error())
}

func Markdown(err error) []byte {
	if err == nil {
		return nil
	}
	switch err := err.(type) {
	case MarkdownError:
		return err.Markdown()
	case interface{ Unwrap() []error }:
		return appendLines(nil, err.Unwrap(), 0, appendLineMd)
	default:
		return []byte(err.Error())
	}
}
