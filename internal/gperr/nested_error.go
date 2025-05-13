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

var emptyError = errStr("")

func (err nestedError) Subject(subject string) Error {
	if err.Err == nil {
		err.Err = PrependSubject(subject, emptyError)
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
var spaces = []byte("                            ")

type appendLineFunc func(buf []byte, err error, level int) []byte

func (err *nestedError) fmtError(appendLine appendLineFunc) []byte {
	if err == nil {
		return appendLine(nil, nilError, 0)
	}
	if err.Err != nil {
		buf := appendLine(nil, err.Err, 0)
		if len(err.Extras) > 0 {
			buf = append(buf, '\n')
			buf = appendLines(buf, err.Extras, 1, appendLine)
		}
		return buf
	}
	return appendLines(nil, err.Extras, 0, appendLine)
}

func (err *nestedError) Error() string {
	return string(err.fmtError(appendLineNormal))
}

func (err *nestedError) Plain() []byte {
	return err.fmtError(appendLinePlain)
}

func (err *nestedError) Markdown() []byte {
	return err.fmtError(appendLineMd)
}

func appendLine(buf []byte, err error, level int, prefix []byte, format func(err error) []byte) []byte {
	if err == nil {
		return appendLine(buf, nilError, level, prefix, format)
	}
	if level == 0 {
		return append(buf, format(err)...)
	}
	buf = append(buf, spaces[:2*level]...)
	buf = append(buf, prefix...)
	buf = append(buf, format(err)...)
	return buf
}

func appendLineNormal(buf []byte, err error, level int) []byte {
	return appendLine(buf, err, level, bulletPrefix, Normal)
}

func appendLinePlain(buf []byte, err error, level int) []byte {
	return appendLine(buf, err, level, bulletPrefix, Plain)
}

func appendLineMd(buf []byte, err error, level int) []byte {
	return appendLine(buf, err, level, markdownBulletPrefix, Markdown)
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
			if err == nil {
				continue
			}
			buf = appendLine(buf, err, level)
			buf = append(buf, '\n')
		}
	}
	return buf
}
