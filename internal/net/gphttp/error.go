package gphttp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"syscall"

	"github.com/yusing/go-proxy/internal/gperr"
	"github.com/yusing/go-proxy/internal/net/gphttp/httpheaders"
)

// ServerError is for handling server errors.
//
// It logs the error and returns http.StatusInternalServerError to the client.
// Status code can be specified as an argument.
func ServerError(w http.ResponseWriter, r *http.Request, err error, code ...int) {
	switch {
	case err == nil,
		errors.Is(err, context.Canceled),
		errors.Is(err, syscall.EPIPE),
		errors.Is(err, syscall.ECONNRESET):
		return
	}
	LogError(r).Msg(err.Error())
	if httpheaders.IsWebsocket(r.Header) {
		return
	}
	if len(code) == 0 {
		code = []int{http.StatusInternalServerError}
	}
	http.Error(w, http.StatusText(code[0]), code[0])
}

// ClientError is for responding to client errors.
//
// It returns http.StatusBadRequest with reason to the client.
// Status code can be specified as an argument.
//
// For JSON marshallable errors (e.g. gperr.Error), it returns the error details as JSON.
// Otherwise, it returns the error details as plain text.
func ClientError(w http.ResponseWriter, r *http.Request, err error, code ...int) {
	if len(code) == 0 {
		code = []int{http.StatusBadRequest}
	}
	w.WriteHeader(code[0])
	accept := GetAccept(r.Header)
	switch {
	case accept.AcceptJSON():
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(err)
	case accept.AcceptMarkdown():
		w.Header().Set("Content-Type", "text/markdown")
		w.Write(gperr.Markdown(err))
	default:
		w.Header().Set("Content-Type", "text/plain")
		w.Write(gperr.Plain(err))
	}
}

// JSONError returns a JSON response of gperr.Error with the given status code.
func JSONError(w http.ResponseWriter, err gperr.Error, code int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(err)
}

// BadRequest returns a Bad Request response with the given error message.
func BadRequest(w http.ResponseWriter, err string, code ...int) {
	if len(code) == 0 {
		code = []int{http.StatusBadRequest}
	}
	w.WriteHeader(code[0])
	w.Write([]byte(err))
}

// Unauthorized returns an Unauthorized response with the given error message.
func Unauthorized(w http.ResponseWriter, err string) {
	BadRequest(w, err, http.StatusUnauthorized)
}

// Forbidden returns a Forbidden response with the given error message.
func Forbidden(w http.ResponseWriter, err string) {
	BadRequest(w, err, http.StatusForbidden)
}

// NotFound returns a Not Found response with the given error message.
func NotFound(w http.ResponseWriter, err string) {
	BadRequest(w, err, http.StatusNotFound)
}

func MissingKey(w http.ResponseWriter, k string) {
	BadRequest(w, k+" is required", http.StatusBadRequest)
}

func InvalidKey(w http.ResponseWriter, k string) {
	BadRequest(w, k+" is invalid", http.StatusBadRequest)
}

func KeyAlreadyExists(w http.ResponseWriter, k, v string) {
	BadRequest(w, fmt.Sprintf("%s %q already exists", k, v), http.StatusBadRequest)
}

func ValueNotFound(w http.ResponseWriter, k, v string) {
	BadRequest(w, fmt.Sprintf("%s %q not found", k, v), http.StatusNotFound)
}
