package gperr

import (
	"errors"
	"fmt"

	"encoding/json"
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

func (err baseError) With(extra error) Error {
	return &nestedError{&err, []error{extra}}
}

func (err baseError) Withf(format string, args ...any) Error {
	return &nestedError{&err, []error{fmt.Errorf(format, args...)}}
}

func (err *baseError) Error() string {
	return err.Err.Error()
}

func (err *baseError) MarshalJSON() ([]byte, error) {
	return json.Marshal(err.Err)
}
