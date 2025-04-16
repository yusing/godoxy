package strutils

import (
	"reflect"
	"strconv"
)

type Parser interface {
	Parse(value string) error
}

func Parse[T Parser](from string) (t T, err error) {
	tt := reflect.TypeOf(t)
	if tt.Kind() == reflect.Ptr {
		t = reflect.New(tt.Elem()).Interface().(T)
	}
	err = t.Parse(from)
	return t, err
}

func MustParse[T Parser](from string) T {
	t, err := Parse[T](from)
	if err != nil {
		panic("must failed: " + err.Error())
	}
	return t
}

func ParseBool(from string) bool {
	b, err := strconv.ParseBool(from)
	if err != nil {
		return false
	}
	return b
}
