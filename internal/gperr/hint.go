package gperr

import "github.com/yusing/goutils/strings/ansi"

type Hint struct {
	Prefix  string
	Message string
	Suffix  string
}

var _ PlainError = (*Hint)(nil)
var _ MarkdownError = (*Hint)(nil)

func (h *Hint) Error() string {
	return h.Prefix + ansi.Info(h.Message) + h.Suffix
}

func (h *Hint) Plain() []byte {
	return []byte(h.Prefix + h.Message + h.Suffix)
}

func (h *Hint) Markdown() []byte {
	return []byte(h.Prefix + "**" + h.Message + "**" + h.Suffix)
}

func (h *Hint) MarshalText() ([]byte, error) {
	return h.Plain(), nil
}

func (h *Hint) String() string {
	return h.Error()
}

func DoYouMean(s string) error {
	if s == "" {
		return nil
	}
	return &Hint{
		Prefix:  "Do you mean ",
		Message: s,
		Suffix:  "?",
	}
}
