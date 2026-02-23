package rules

import (
	"net/http"

	httputils "github.com/yusing/goutils/http"
)

type (
	CheckFunc func(w *httputils.ResponseModifier, r *http.Request) bool
	Checker   interface {
		Check(w *httputils.ResponseModifier, r *http.Request) bool
	}
	CheckMatchSingle []Checker
	CheckMatchAll    []Checker
)

func (checker CheckFunc) Check(w *httputils.ResponseModifier, r *http.Request) bool {
	return checker(w, r)
}

func (checkers CheckMatchSingle) Check(w *httputils.ResponseModifier, r *http.Request) bool {
	for _, check := range checkers {
		if check.Check(w, r) {
			return true
		}
	}
	return false
}

func (checkers CheckMatchAll) Check(w *httputils.ResponseModifier, r *http.Request) bool {
	for _, check := range checkers {
		if !check.Check(w, r) {
			return false
		}
	}
	return true
}
