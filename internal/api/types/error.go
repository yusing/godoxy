package apitypes

import (
	"errors"

	"github.com/yusing/go-proxy/internal/gperr"
)

type ErrorResponse struct {
	Message string `json:"message"`
	Error   string `json:"error,omitempty" extensions:"x-nullable"`
} // @name ErrorResponse

type serverError struct {
	Message string
	Err     error
}

// Error returns a generic error response
func Error(message string, err ...error) ErrorResponse {
	if len(err) > 0 {
		var gpErr gperr.Error
		if errors.As(err[0], &gpErr) {
			return ErrorResponse{
				Message: message,
				Error:   string(gpErr.Plain()),
			}
		}
		return ErrorResponse{
			Message: message,
			Error:   err[0].Error(),
		}
	}
	return ErrorResponse{
		Message: message,
	}
}

func InternalServerError(err error, message string) error {
	return serverError{
		Message: message,
		Err:     err,
	}
}

func (e serverError) Error() string {
	if e.Err != nil {
		return e.Message + ": " + e.Err.Error()
	}
	return e.Message
}

func (e serverError) Unwrap() error {
	return e.Err
}
