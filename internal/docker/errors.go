package docker

import (
	"encoding/json"

	"github.com/yusing/go-proxy/internal/gperr"
)

type containerError struct {
	errs *gperr.Builder
}

func (e *containerError) Add(err error) {
	if e.errs == nil {
		e.errs = gperr.NewBuilder()
	}
	e.errs.Add(err)
}

func (e *containerError) Error() string {
	if e.errs == nil {
		return "<niL>"
	}
	return e.errs.String()
}

func (e *containerError) Unwrap() error {
	return e.errs.Error()
}

func (e *containerError) MarshalJSON() ([]byte, error) {
	err := e.errs.Error().(interface{ Plain() []byte })
	return json.Marshal(string(err.Plain()))
}
