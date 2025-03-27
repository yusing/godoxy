package rules

import (
	"github.com/yusing/go-proxy/internal/gperr"
)

var (
	ErrUnterminatedQuotes     = gperr.New("unterminated quotes")
	ErrUnsupportedEscapeChar  = gperr.New("unsupported escape char")
	ErrUnknownDirective       = gperr.New("unknown directive")
	ErrInvalidArguments       = gperr.New("invalid arguments")
	ErrInvalidOnTarget        = gperr.New("invalid `rule.on` target")
	ErrInvalidCommandSequence = gperr.New("invalid command sequence")
	ErrInvalidSetTarget       = gperr.New("invalid `rule.set` target")

	ErrExpectNoArg       = gperr.Wrap(ErrInvalidArguments, "expect no arg")
	ErrExpectOneArg      = gperr.Wrap(ErrInvalidArguments, "expect 1 arg")
	ErrExpectTwoArgs     = gperr.Wrap(ErrInvalidArguments, "expect 2 args")
	ErrExpectKVOptionalV = gperr.Wrap(ErrInvalidArguments, "expect 'key' or 'key value'")
)
