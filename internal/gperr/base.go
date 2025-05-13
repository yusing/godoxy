package gperr

import (
	"encoding/json"
	"errors"
	"fmt"
)

// baseError is an immutable wrapper around an error.
//
//nolint:recvcheck
type baseError struct {
	Err error `json:"err"`
}

func (err *baseError) Unwrap() error {
	return err.Err
}

func (err *baseError) Is(other error) bool {
	if other, ok := other.(*baseError); ok {
		return errors.Is(err.Err, other.Err)
	}
	return errors.Is(err.Err, other)
}

func (err baseError) Subject(subject string) Error {
	err.Err = PrependSubject(subject, err.Err)
	return &err
}

func (err *baseError) Subjectf(format string, args ...any) Error {
	if len(args) > 0 {
		return err.Subject(fmt.Sprintf(format, args...))
	}
	return err.Subject(format)
}

func (err *baseError) With(extra error) Error {
	if extra == nil {
		return err
	}
	return &nestedError{&baseError{err.Err}, []error{extra}}
}

func (err baseError) Withf(format string, args ...any) Error {
	return &nestedError{&err, []error{fmt.Errorf(format, args...)}}
}

func (err *baseError) Error() string {
	return err.Err.Error()
}

// MarshalJSON implements the json.Marshaler interface.
func (err *baseError) MarshalJSON() ([]byte, error) {
	//nolint:errorlint
	switch err := err.Err.(type) {
	case Error, *withSubject:
		return json.Marshal(err)
	case json.Marshaler:
		return err.MarshalJSON()
	case interface{ MarshalText() ([]byte, error) }:
		return err.MarshalText()
	default:
		return json.Marshal(err.Error())
	}
}

func (err *baseError) Plain() []byte {
	return Plain(err.Err)
}

func (err *baseError) Markdown() []byte {
	return Markdown(err.Err)
}
