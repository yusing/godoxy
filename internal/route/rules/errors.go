package rules

import (
	gperr "github.com/yusing/goutils/errs"
)

var (
	ErrUnterminatedQuotes     = gperr.New("unterminated quotes")
	ErrUnterminatedBrackets   = gperr.New("unterminated brackets")
	ErrUnterminatedEnvVar     = gperr.New("unterminated env var")
	ErrUnknownDirective       = gperr.New("unknown directive")
	ErrUnknownModField        = gperr.New("unknown field")
	ErrEnvVarNotFound         = gperr.New("env variable not found")
	ErrInvalidArguments       = gperr.New("invalid arguments")
	ErrInvalidOnTarget        = gperr.New("invalid `rule.on` target")
	ErrInvalidCommandSequence = gperr.New("invalid command sequence")

	ErrExpectNoArg       = gperr.Wrap(ErrInvalidArguments, "expect no arg")
	ErrExpectOneArg      = gperr.Wrap(ErrInvalidArguments, "expect 1 arg")
	ErrExpectTwoArgs     = gperr.Wrap(ErrInvalidArguments, "expect 2 args")
	ErrExpectThreeArgs   = gperr.Wrap(ErrInvalidArguments, "expect 3 args")
	ErrExpectFourArgs    = gperr.Wrap(ErrInvalidArguments, "expect 4 args")
	ErrExpectKVOptionalV = gperr.Wrap(ErrInvalidArguments, "expect 'key' or 'key value'")

	errTerminated = gperr.New("terminated")
)
