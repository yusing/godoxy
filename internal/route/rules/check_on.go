package rules

import "net/http"

type (
	CheckFunc func(w http.ResponseWriter, r *http.Request) bool
	Checker   interface {
		Check(w http.ResponseWriter, r *http.Request) bool
	}
	CheckMatchSingle []Checker
	CheckMatchAll    []Checker
)

func (checker CheckFunc) Check(w http.ResponseWriter, r *http.Request) bool {
	return checker(w, r)
}

func (checkers CheckMatchSingle) Check(w http.ResponseWriter, r *http.Request) bool {
	for _, check := range checkers {
		if check.Check(w, r) {
			return true
		}
	}
	return false
}

func (checkers CheckMatchAll) Check(w http.ResponseWriter, r *http.Request) bool {
	for _, check := range checkers {
		if !check.Check(w, r) {
			return false
		}
	}
	return true
}
