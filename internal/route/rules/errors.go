package rules

import (
	gperr "github.com/yusing/goutils/errs"
)

var (
	ErrUnterminatedQuotes      = gperr.New("unterminated quotes")
	ErrUnterminatedBrackets    = gperr.New("unterminated brackets")
	ErrUnterminatedParenthesis = gperr.New("unterminated parenthesis")
	ErrUnterminatedEnvVar      = gperr.New("unterminated env var")
	ErrUnknownDirective        = gperr.New("unknown directive")
	ErrUnknownModField         = gperr.New("unknown field")
	ErrEnvVarNotFound          = gperr.New("env variable not found")
	ErrInvalidArguments        = gperr.New("invalid arguments")
	ErrInvalidOnTarget         = gperr.New("invalid `rule.on` target")

	ErrMultipleDefaultRules = gperr.New("multiple default rules")
	ErrDeadRule             = gperr.New("dead rule")

	// vars errors
	ErrNoArgProvided   = gperr.New("no argument provided")
	ErrUnexpectedVar   = gperr.New("unexpected variable")
	ErrUnexpectedQuote = gperr.New("unexpected quote")

	ErrExpectNoArg          = gperr.Wrap(ErrInvalidArguments, "expect no arg")
	ErrExpectOneArg         = gperr.Wrap(ErrInvalidArguments, "expect 1 arg")
	ErrExpectOneOrTwoArgs   = gperr.Wrap(ErrInvalidArguments, "expect 1 or 2 args")
	ErrExpectTwoArgs        = gperr.Wrap(ErrInvalidArguments, "expect 2 args")
	ErrExpectTwoOrThreeArgs = gperr.Wrap(ErrInvalidArguments, "expect 2 or 3 args")
	ErrExpectThreeArgs      = gperr.Wrap(ErrInvalidArguments, "expect 3 args")
	ErrExpectFourArgs       = gperr.Wrap(ErrInvalidArguments, "expect 4 args")
	ErrExpectKVOptionalV    = gperr.Wrap(ErrInvalidArguments, "expect 'key' or 'key value'")

	ErrInvalidBlockSyntax = gperr.New("invalid block syntax") // TODO: struct this error
)
