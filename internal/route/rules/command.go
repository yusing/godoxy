package rules

import (
	"errors"
	"net/http"

	httputils "github.com/yusing/goutils/http"
)

var errTerminateRule = errors.New("terminate rule")

type (
	HandlerFunc func(w *httputils.ResponseModifier, r *http.Request, upstream http.HandlerFunc) error
	Handler     struct {
		fn        HandlerFunc
		phase     PhaseFlag
		terminate bool
	}

	CommandHandler interface {
		// CommandHandler can read and modify the values
		// then handle the request
		// finally proceed to next command (or return) base on situation
		ServeHTTP(w *httputils.ResponseModifier, r *http.Request, upstream http.HandlerFunc) error
		Phase() PhaseFlag
	}

	// Commands is a slice of CommandHandler.
	Commands []CommandHandler
)

func (h Handler) ServeHTTP(w *httputils.ResponseModifier, r *http.Request, upstream http.HandlerFunc) error {
	return h.fn(w, r, upstream)
}

func (h Handler) Phase() PhaseFlag {
	return h.phase
}

func (h Handler) Terminates() bool {
	return h.terminate
}

func (c Commands) ServeHTTP(w *httputils.ResponseModifier, r *http.Request, upstream http.HandlerFunc) error {
	for _, cmd := range c {
		err := cmd.ServeHTTP(w, r, upstream)
		if err != nil {
			// Terminating actions stop the command chain immediately.
			// Will be handled by the caller.
			return err
		}
	}
	return nil
}

func (c Commands) Phase() PhaseFlag {
	req := PhaseNone
	for _, cmd := range c {
		req |= cmd.Phase()
	}
	return req
}
