package gperr

import (
	"errors"
	"fmt"
)

//nolint:recvcheck
type nestedError struct {
	Err    error   `json:"err"`
	Extras []error `json:"extras"`
}

func (err nestedError) Subject(subject string) Error {
	if err.Err == nil {
		err.Err = PrependSubject(subject, errStr(""))
	} else {
		err.Err = PrependSubject(subject, err.Err)
	}
	return &err
}

func (err *nestedError) Subjectf(format string, args ...any) Error {
	if len(args) > 0 {
		return err.Subject(fmt.Sprintf(format, args...))
	}
	return err.Subject(format)
}

func (err nestedError) With(extra error) Error {
	if extra != nil {
		err.Extras = append(err.Extras, extra)
	}
	return &err
}

func (err nestedError) Withf(format string, args ...any) Error {
	if len(args) > 0 {
		err.Extras = append(err.Extras, fmt.Errorf(format, args...))
	} else {
		err.Extras = append(err.Extras, newError(format))
	}
	return &err
}

func (err *nestedError) Unwrap() []error {
	if err.Err == nil {
		if len(err.Extras) == 0 {
			return nil
		}
		return err.Extras
	}
	return append([]error{err.Err}, err.Extras...)
}

func (err *nestedError) Is(other error) bool {
	if errors.Is(err.Err, other) {
		return true
	}
	for _, e := range err.Extras {
		if errors.Is(e, other) {
			return true
		}
	}
	return false
}

var nilError = newError("<nil>")
var bulletPrefix = []byte("• ")
var markdownBulletPrefix = []byte("- ")
var spaces = []byte("                ")

type appendLineFunc func(buf []byte, err error, level int) []byte

func (err *nestedError) Error() string {
	if err.Err == nil {
		return nilError.Error()
	}
	buf := appendLineNormal(nil, err.Err, 0)
	if len(err.Extras) > 0 {
		buf = append(buf, '\n')
		buf = appendLines(buf, err.Extras, 1, appendLineNormal)
	}
	return string(buf)
}

func (err *nestedError) Plain() []byte {
	if err.Err == nil {
		return appendLinePlain(nil, nilError, 0)
	}
	buf := appendLinePlain(nil, err.Err, 0)
	if len(err.Extras) > 0 {
		buf = append(buf, '\n')
		buf = appendLines(buf, err.Extras, 1, appendLinePlain)
	}
	return buf
}

func (err *nestedError) Markdown() []byte {
	if err.Err == nil {
		return appendLineMd(nil, nilError, 0)
	}

	buf := appendLineMd(nil, err.Err, 0)
	if len(err.Extras) > 0 {
		buf = append(buf, '\n')
		buf = appendLines(buf, err.Extras, 1, appendLineMd)
	}
	return buf
}

func appendLineNormal(buf []byte, err error, level int) []byte {
	if level == 0 {
		return append(buf, err.Error()...)
	}
	buf = append(buf, spaces[:2*level]...)
	buf = append(buf, bulletPrefix...)
	buf = append(buf, err.Error()...)
	return buf
}

func appendLinePlain(buf []byte, err error, level int) []byte {
	if level == 0 {
		return append(buf, Plain(err)...)
	}
	buf = append(buf, spaces[:2*level]...)
	buf = append(buf, bulletPrefix...)
	buf = append(buf, Plain(err)...)
	return buf
}

func appendLineMd(buf []byte, err error, level int) []byte {
	if level == 0 {
		return append(buf, Markdown(err)...)
	}
	buf = append(buf, spaces[:2*level]...)
	buf = append(buf, markdownBulletPrefix...)
	buf = append(buf, Markdown(err)...)
	return buf
}

func appendLines(buf []byte, errs []error, level int, appendLine appendLineFunc) []byte {
	if len(errs) == 0 {
		return buf
	}
	for _, err := range errs {
		switch err := wrap(err).(type) {
		case *nestedError:
			if err.Err != nil {
				buf = appendLine(buf, err.Err, level)
				buf = append(buf, '\n')
				buf = appendLines(buf, err.Extras, level+1, appendLine)
			} else {
				buf = appendLines(buf, err.Extras, level, appendLine)
			}
		default:
			buf = appendLine(buf, err, level)
			buf = append(buf, '\n')
		}
	}
	return buf
}
