package accesslog

import (
	"strconv"

	"github.com/yusing/go-proxy/internal/gperr"
	"github.com/yusing/go-proxy/internal/utils/strutils"
)

type StatusCodeRange struct {
	Start int
	End   int
}

var ErrInvalidStatusCodeRange = gperr.New("invalid status code range")

func (r *StatusCodeRange) Includes(code int) bool {
	return r.Start <= code && code <= r.End
}

// Parse implements strutils.Parser.
func (r *StatusCodeRange) Parse(v string) error {
	split := strutils.SplitRune(v, '-')
	switch len(split) {
	case 1:
		start, err := strconv.Atoi(split[0])
		if err != nil {
			return gperr.Wrap(err)
		}
		r.Start = start
		r.End = start
		return nil
	case 2:
		start, errStart := strconv.Atoi(split[0])
		end, errEnd := strconv.Atoi(split[1])
		if err := gperr.Join(errStart, errEnd); err != nil {
			return err
		}
		r.Start = start
		r.End = end
		return nil
	default:
		return ErrInvalidStatusCodeRange.Subject(v)
	}
}

func (r *StatusCodeRange) String() string {
	if r.Start == r.End {
		return strconv.Itoa(r.Start)
	}
	return strconv.Itoa(r.Start) + "-" + strconv.Itoa(r.End)
}
