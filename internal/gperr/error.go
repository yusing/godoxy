package gperr

type Error interface {
	error

	// Is is a wrapper for errors.Is when there is no sub-error.
	//
	// When there are sub-errors, they will also be checked.
	Is(other error) bool
	// With appends a sub-error to the error.
	With(extra error) Error
	// Withf is a wrapper for With(fmt.Errorf(format, args...)).
	Withf(format string, args ...any) Error
	// Subject prepends the given subject with a colon and space to the error message.
	//
	// If there is already a subject in the error message, the subject will be
	// prepended to the existing subject with " > ".
	//
	// Subject empty string is ignored.
	Subject(subject string) Error
	// Subjectf is a wrapper for Subject(fmt.Sprintf(format, args...)).
	Subjectf(format string, args ...any) Error
	PlainError
	MarkdownError
}

type PlainError interface {
	Plain() []byte
}

type MarkdownError interface {
	Markdown() []byte
}

// this makes JSON marshaling work,
// as the builtin one doesn't.
//
//nolint:errname
type errStr string

func (err errStr) Error() string {
	return string(err)
}
