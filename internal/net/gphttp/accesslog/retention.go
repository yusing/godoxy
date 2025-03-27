package accesslog

import (
	"strconv"

	"github.com/yusing/go-proxy/internal/gperr"
	"github.com/yusing/go-proxy/internal/utils/strutils"
)

type Retention struct {
	Days uint64 `json:"days"`
	Last uint64 `json:"last"`
}

var (
	ErrInvalidSyntax = gperr.New("invalid syntax")
	ErrZeroValue     = gperr.New("zero value")
)

var defaultChunkSize = 64 * 1024 // 64KB

// Syntax:
//
// <N> days|weeks|months
//
// last <N>
//
// Parse implements strutils.Parser.
func (r *Retention) Parse(v string) (err error) {
	split := strutils.SplitSpace(v)
	if len(split) != 2 {
		return ErrInvalidSyntax.Subject(v)
	}
	switch split[0] {
	case "last":
		r.Last, err = strconv.ParseUint(split[1], 10, 64)
	default: // <N> days|weeks|months
		r.Days, err = strconv.ParseUint(split[0], 10, 64)
		if err != nil {
			return
		}
		switch split[1] {
		case "days":
		case "weeks":
			r.Days *= 7
		case "months":
			r.Days *= 30
		default:
			return ErrInvalidSyntax.Subject("unit " + split[1])
		}
	}
	if r.Days == 0 && r.Last == 0 {
		return ErrZeroValue
	}
	return
}
