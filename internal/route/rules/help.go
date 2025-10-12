package rules

import (
	"strings"

	gperr "github.com/yusing/goutils/errs"
)

type Help struct {
	command     string
	description string
	args        map[string]string // args[arg] -> description
}

/*
Generate help string as error, e.g.

	 rewrite <from> <to>
		from: the path to rewrite, must start with /
		to: the path to rewrite to, must start with /
*/
func (h *Help) Error() gperr.Error {
	errs := gperr.Multiline()
	errs.Adds(h.command)
	if h.description != "" {
		for line := range strings.SplitSeq(h.description, "\n") {
			errs.Adds(line)
		}
	}
	for arg, desc := range h.args {
		errs.AddLinesString(arg + ": " + desc)
	}
	return errs
}
