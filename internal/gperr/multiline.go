package gperr

import (
	"fmt"
	"reflect"
)

type MultilineError struct {
	nestedError
}

func Multiline() *MultilineError {
	return &MultilineError{}
}

func (m *MultilineError) add(err error) {
	if err == nil {
		return
	}
	m.Extras = append(m.Extras, err)
}

func (m *MultilineError) Addf(format string, args ...any) *MultilineError {
	m.add(fmt.Errorf(format, args...))
	return m
}

func (m *MultilineError) Adds(s string) *MultilineError {
	m.add(newError(s))
	return m
}

func (m *MultilineError) AddLines(lines ...any) *MultilineError {
	v := reflect.ValueOf(lines)
	if v.Kind() == reflect.Slice {
		for i := range v.Len() {
			switch v := v.Index(i).Interface().(type) {
			case string:
				m.add(newError(v))
			case error:
				m.add(v)
			default:
				m.add(fmt.Errorf("%v", v))
			}
		}
	}
	return m
}

func (m *MultilineError) AddLinesString(lines ...string) *MultilineError {
	for _, line := range lines {
		m.add(newError(line))
	}
	return m
}
