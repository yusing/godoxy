package gperr

import (
	"errors"
	"slices"
	"strings"

	"github.com/yusing/go-proxy/pkg/json"
	"github.com/yusing/go-proxy/internal/utils/strutils/ansi"
)

//nolint:errname
type withSubject struct {
	Subjects []string
	Err      error

	pendingSubject string
}

const subjectSep = " > "

func highlight(subject string) string {
	return ansi.HighlightRed + subject + ansi.Reset
}

func PrependSubject(subject string, err error) error {
	if err == nil {
		return nil
	}

	if subject == "" {
		return err
	}

	//nolint:errorlint
	switch err := err.(type) {
	case *withSubject:
		return err.Prepend(subject)
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
	// subject is in reversed order
	n := len(err.Subjects)
	size := 0
	errStr := err.Err.Error()
	var sb strings.Builder
	for _, s := range err.Subjects {
		size += len(s)
	}
	sb.Grow(size + 2 + n*len(subjectSep) + len(errStr) + len(highlight("")))

	for i := n - 1; i > 0; i-- {
		sb.WriteString(err.Subjects[i])
		sb.WriteString(subjectSep)
	}
	sb.WriteString(highlight(err.Subjects[0]))
	sb.WriteString(": ")
	sb.WriteString(errStr)
	return sb.String()
}

func (err *withSubject) MarshalJSONTo(buf []byte) []byte {
	subjects := slices.Clone(err.Subjects)
	slices.Reverse(subjects)

	reversed := map[string]any{
		"subjects": subjects,
		"err":      err.Err,
	}
	return json.MarshalTo(reversed, buf)
}
