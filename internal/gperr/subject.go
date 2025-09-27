package gperr

import (
	"bytes"
	"encoding/json"
	"errors"
	"slices"

	"github.com/yusing/goutils/strings/ansi"
)

//nolint:errname
type withSubject struct {
	Subjects []string
	Err      error

	pendingSubject string
}

const subjectSep = " > "

type highlightFunc func(subject string) string

var _ PlainError = (*withSubject)(nil)
var _ MarkdownError = (*withSubject)(nil)

func highlightANSI(subject string) string {
	return ansi.HighlightRed + subject + ansi.Reset
}

func highlightMarkdown(subject string) string {
	return "**" + subject + "**"
}

func noHighlight(subject string) string {
	return subject
}

func PrependSubject(subject string, err error) error {
	if err == nil {
		return nil
	}

	//nolint:errorlint
	switch err := err.(type) {
	case *withSubject:
		return err.Prepend(subject)
	case *wrappedError:
		return &wrappedError{
			Err:     PrependSubject(subject, err.Err),
			Message: err.Message,
		}
	case Error:
		return err.Subject(subject)
	}
	return &withSubject{[]string{subject}, err, ""}
}

func (err *withSubject) Prepend(subject string) *withSubject {
	if subject == "" {
		return err
	}

	clone := *err
	switch subject[0] {
	case '[', '(', '{':
		// since prepend is called in depth-first order,
		// the subject of the index is not yet seen
		// add it when the next subject is seen
		clone.pendingSubject += subject
	default:
		clone.Subjects = append(clone.Subjects, subject)
		if clone.pendingSubject != "" {
			clone.Subjects[len(clone.Subjects)-1] = subject + clone.pendingSubject
			clone.pendingSubject = ""
		}
	}
	return &clone
}

func (err *withSubject) Is(other error) bool {
	return errors.Is(other, err.Err)
}

func (err *withSubject) Unwrap() error {
	return err.Err
}

func (err *withSubject) Error() string {
	return string(err.fmtError(highlightANSI))
}

func (err *withSubject) Plain() []byte {
	return err.fmtError(noHighlight)
}

func (err *withSubject) Markdown() []byte {
	return err.fmtError(highlightMarkdown)
}

func (err *withSubject) fmtError(highlight highlightFunc) []byte {
	// subject is in reversed order
	size := 0
	errStr := err.Err.Error()
	subjects := err.Subjects
	if err.pendingSubject != "" {
		subjects = append(subjects, err.pendingSubject)
	}
	var buf bytes.Buffer
	for _, s := range subjects {
		size += len(s)
	}
	n := len(subjects)
	buf.Grow(size + 2 + n*len(subjectSep) + len(errStr) + len(highlight("")))

	for i := n - 1; i > 0; i-- {
		buf.WriteString(subjects[i])
		buf.WriteString(subjectSep)
	}
	buf.WriteString(highlight(subjects[0]))
	if errStr != "" {
		buf.WriteString(": ")
		buf.WriteString(errStr)
	}
	return buf.Bytes()
}

// MarshalJSON implements the json.Marshaler interface.
func (err *withSubject) MarshalJSON() ([]byte, error) {
	subjects := slices.Clone(err.Subjects)
	slices.Reverse(subjects)
	reversed := struct {
		Subjects []string `json:"subjects"`
		Err      error    `json:"err"`
	}{
		Subjects: subjects,
		Err:      err.Err,
	}
	if err.pendingSubject != "" {
		reversed.Subjects = append(reversed.Subjects, err.pendingSubject)
	}

	return json.Marshal(reversed)
}
