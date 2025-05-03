package gperr

import (
	"errors"
	"fmt"
)

type wrappedError struct {
	Err     error
	Message string
}

var _ PlainError = (*wrappedError)(nil)
var _ MarkdownError = (*wrappedError)(nil)

func (e *wrappedError) Error() string {
	return fmt.Sprintf("%s: %s", e.Message, e.Err.Error())
}

func (e *wrappedError) Plain() []byte {
	return fmt.Appendf(nil, "%s: %s", e.Message, e.Err.Error())
}

func (e *wrappedError) Markdown() []byte {
	return fmt.Appendf(nil, "**%s**: %s", e.Message, e.Err.Error())
}

func (e *wrappedError) Unwrap() error {
	return e.Err
}

func (e *wrappedError) Is(target error) bool {
	return errors.Is(e.Err, target)
}
