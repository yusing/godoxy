package accesslog

import (
	"fmt"
	"strconv"

	"github.com/yusing/go-proxy/internal/gperr"
	"github.com/yusing/go-proxy/internal/utils/strutils"
)

type Retention struct {
	Days     uint64 `json:"days"`
	Last     uint64 `json:"last"`
	KeepSize uint64 `json:"keep_size"`
} // @name LogRetention

var (
	ErrInvalidSyntax = gperr.New("invalid syntax")
	ErrZeroValue     = gperr.New("zero value")
)

// see back_scanner_test.go#L210 for benchmarks
var defaultChunkSize = 256 * kilobyte

// Syntax:
//
// <N> days|weeks|months
//
// last <N>
//
// <N> KB|MB|GB|kb|mb|gb
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
		n, err := strconv.ParseUint(split[0], 10, 64)
		if err != nil {
			return err
		}
		switch split[1] {
		case "day", "days":
			r.Days = n
		case "week", "weeks":
			r.Days = n * 7
		case "month", "months":
			r.Days = n * 30
		case "kb", "Kb":
			r.KeepSize = n * kilobits
		case "KB":
			r.KeepSize = n * kilobyte
		case "mb", "Mb":
			r.KeepSize = n * megabits
		case "MB":
			r.KeepSize = n * megabyte
		case "gb", "Gb":
			r.KeepSize = n * gigabits
		case "GB":
			r.KeepSize = n * gigabyte
		default:
			return ErrInvalidSyntax.Subject("unit " + split[1])
		}
	}
	if !r.IsValid() {
		return ErrZeroValue
	}
	return
}

func (r *Retention) String() string {
	if r.Days > 0 {
		return fmt.Sprintf("%d days", r.Days)
	}
	if r.Last > 0 {
		return fmt.Sprintf("last %d", r.Last)
	}
	if r.KeepSize > 0 {
		return strutils.FormatByteSize(r.KeepSize)
	}
	return "<invalid>"
}

func (r *Retention) IsValid() bool {
	if r == nil {
		return false
	}
	return r.Days > 0 || r.Last > 0 || r.KeepSize > 0
}
