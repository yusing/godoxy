package rules

import (
	"errors"
	"fmt"
	"net/http"
	"reflect"
	"slices"
	"strings"
	"unicode"

	"github.com/goccy/go-yaml"
	"github.com/quic-go/quic-go/http3"
	"github.com/rs/zerolog/log"
	"github.com/yusing/godoxy/internal/serialization"
	gperr "github.com/yusing/goutils/errs"
	httputils "github.com/yusing/goutils/http"
	"golang.org/x/net/http2"

	_ "unsafe"
)

type (
	/*
		Rules is a list of rules.

			Example:

				proxy.app1.rules: |
					- name: default
						do: |
							rewrite / /index.html
							serve /var/www/goaccess
					- name: ws
						on: |
							header Connection Upgrade
							header Upgrade websocket
						do: bypass

				proxy.app2.rules: |
					- name: default
						do: bypass
					- name: block POST and PUT
						on: method POST | method PUT
						do: error 403 Forbidden
	*/
	//nolint:recvcheck
	Rules []Rule
	// Rule represents a reverse proxy rule.
	// The `Do` field is executed when `On` matches.
	//
	// - A rule may have multiple lines in the `On` section.
	// - All `On` lines must match for the rule to trigger.
	// - Each line can have several checks—one match per line is enough for that line.
	Rule struct {
		Name string  `json:"name"`
		On   RuleOn  `json:"on" swaggertype:"string"`
		Do   Command `json:"do" swaggertype:"string"`
	}
)

func isDefaultRule(rule Rule) bool {
	return rule.Name == "default" || rule.On.raw == OnDefault
}

func (rules Rules) Validate() gperr.Error {
	var defaultRulesFound []int
	for i := range rules {
		rule := rules[i]
		if isDefaultRule(rule) {
			defaultRulesFound = append(defaultRulesFound, i)
		}
		if rules[i].Name == "" {
			// set name to index if name is empty
			rules[i].Name = fmt.Sprintf("rule[%d]", i)
		}
	}
	if len(defaultRulesFound) > 1 {
		return ErrMultipleDefaultRules.Withf("found %d", len(defaultRulesFound))
	}
	for i := range rules {
		r1 := rules[i]
		if isDefaultRule(r1) || r1.On.phase.IsPostRule() || !r1.doesTerminateInPre() {
			continue
		}
		sig1, ok := matcherSignature(r1.On.raw)
		if !ok {
			continue
		}
		for j := i + 1; j < len(rules); j++ {
			r2 := rules[j]
			if isDefaultRule(r2) || r2.On.phase.IsPostRule() {
				continue
			}
			sig2, ok := matcherSignature(r2.On.raw)
			if !ok || sig1 != sig2 {
				continue
			}
			return ErrDeadRule.Withf("rule[%d] shadows rule[%d] with same matcher", i, j)
		}
	}
	return nil
}

func (rule Rule) doesTerminateInPre() bool {
	return commandsTerminateInPre(rule.Do.pre)
}

func commandsTerminateInPre(cmds []CommandHandler) bool {
	return slices.ContainsFunc(cmds, commandTerminatesInPre)
}

func commandTerminatesInPre(cmd CommandHandler) bool {
	switch c := cmd.(type) {
	case Handler:
		return c.Terminates()
	case *Handler:
		return c.Terminates()
	case IfBlockCommand:
		return ruleOnAlwaysTrue(c.On) && commandsTerminateInPre(c.Do)
	case *IfBlockCommand:
		return c != nil && ruleOnAlwaysTrue(c.On) && commandsTerminateInPre(c.Do)
	case IfElseBlockCommand:
		return ifElseBlockTerminatesInPre(c)
	case *IfElseBlockCommand:
		return c != nil && ifElseBlockTerminatesInPre(*c)
	default:
		return false
	}
}

func ifElseBlockTerminatesInPre(cmd IfElseBlockCommand) bool {
	hasFallback := len(cmd.Else) > 0
	for _, br := range cmd.Ifs {
		if !commandsTerminateInPre(br.Do) {
			return false
		}
		if ruleOnAlwaysTrue(br.On) {
			hasFallback = true
		}
	}
	if !hasFallback {
		return false
	}
	if len(cmd.Else) > 0 && !commandsTerminateInPre(cmd.Else) {
		return false
	}
	return true
}

func ruleOnAlwaysTrue(on RuleOn) bool {
	return strings.TrimSpace(on.raw) == OnDefault || on.checker == nil
}

func matcherSignature(raw string) (string, bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "(any)", true // unconditional rule
	}

	andParts := splitAnd(raw)
	if len(andParts) == 0 {
		return "", false
	}

	canonAnd := make([]string, 0, len(andParts))
	for _, andPart := range andParts {
		orParts := splitPipe(andPart)
		if len(orParts) == 0 {
			continue
		}
		canonOr := make([]string, 0, len(orParts))
		for _, atom := range orParts {
			subject, args, err := parse(strings.TrimSpace(atom))
			if err != nil || subject == "" {
				return "", false
			}
			canonOr = append(canonOr, subject+" "+strings.Join(args, "\x00"))
		}
		slices.Sort(canonOr)
		canonOr = slices.Compact(canonOr)
		canonAnd = append(canonAnd, "("+strings.Join(canonOr, "|")+")")
	}

	slices.Sort(canonAnd)
	canonAnd = slices.Compact(canonAnd)
	if len(canonAnd) == 0 {
		return "", false
	}
	return strings.Join(canonAnd, "&"), true
}

// Parse parses a rule configuration string.
// It first tries the block syntax (if the string contains a top-level '{'),
// then falls back to YAML syntax.
func (rules *Rules) Parse(config string) error {
	config = strings.TrimSpace(config)
	if config == "" {
		return nil
	}

	blockTried := false
	var blockErr gperr.Error

	// Prefer block syntax if it looks like block syntax.
	if hasTopLevelLBrace(config) {
		blockTried = true
		blockRules, err := parseBlockRules(config)
		if err == nil {
			*rules = blockRules
			return nil
		}
		blockErr = err
	}

	// YAML fallback
	var anySlice []any
	yamlErr := yaml.Unmarshal([]byte(config), &anySlice)
	if yamlErr == nil {
		return serialization.ConvertSlice(reflect.ValueOf(anySlice), reflect.ValueOf(rules), false)
	}

	// If YAML fails and we haven't tried block syntax yet, try it now.
	if !blockTried {
		blockRules, err := parseBlockRules(config)
		if err == nil {
			*rules = blockRules
			return nil
		}
		blockErr = err
	}
	return blockErr
}

// hasTopLevelLBrace reports whether s contains a '{' outside quotes/backticks and comments.
// Used to decide whether to prioritize the block syntax.
func hasTopLevelLBrace(s string) bool {
	quote := rune(0)
	inLine := false
	inBlock := false

	for i := 0; i < len(s); i++ {
		c := s[i]

		if inLine {
			if c == '\n' {
				inLine = false
			}
			continue
		}
		if inBlock {
			if c == '*' && i+1 < len(s) && s[i+1] == '/' {
				inBlock = false
				i++
			}
			continue
		}

		if quote != 0 {
			if quote != '`' && c == '\\' && i+1 < len(s) {
				i++
				continue
			}
			if rune(c) == quote {
				quote = 0
			}
			continue
		}

		switch c {
		case '\'', '"', '`':
			quote = rune(c)
			continue
		case '{':
			return true
		case '#':
			inLine = true
			continue
		case '/':
			if i+1 < len(s) && s[i+1] == '/' {
				inLine = true
				i++
				continue
			}
			if i+1 < len(s) && s[i+1] == '*' {
				inBlock = true
				i++
				continue
			}
		default:
			if unicode.IsSpace(rune(c)) {
				continue
			}
		}
	}
	return false
}

// BuildHandler returns a http.HandlerFunc that implements the rules.
func (rules Rules) BuildHandler(up http.HandlerFunc) http.HandlerFunc {
	if len(rules) == 0 {
		return up
	}

	var defaultRule *Rule

	var nonDefaultRules Rules
	for _, rule := range rules {
		if isDefaultRule(rule) {
			r := rule
			defaultRule = &r
		} else {
			nonDefaultRules = append(nonDefaultRules, rule)
		}
	}

	if len(nonDefaultRules) == 0 {
		if defaultRule == nil || defaultRule.Do.raw == CommandUpstream {
			return up
		}
	}

	execPreCommand := func(cmd Command, w *httputils.ResponseModifier, r *http.Request) error {
		return cmd.pre.ServeHTTP(w, r, up)
	}

	execPostCommand := func(cmd Command, w *httputils.ResponseModifier, r *http.Request) error {
		return cmd.post.ServeHTTP(w, r, up)
	}

	return func(w http.ResponseWriter, r *http.Request) {
		rm := httputils.NewResponseModifier(w)
		defer func() {
			if _, err := rm.FlushRelease(); err != nil {
				logFlushError(err, r)
			}
		}()

		var hasError bool

		executedPre := make([]bool, len(nonDefaultRules))
		terminatedInPre := make([]bool, len(nonDefaultRules))
		matchedNonDefaultPre := false
		preTerminated := false
		for i, rule := range nonDefaultRules {
			if rule.On.phase.IsPostRule() || !rule.On.Check(rm, r) {
				continue
			}
			matchedNonDefaultPre = true
			if preTerminated {
				// Preserve post-only commands (e.g. logging) even after
				// pre-phase termination.
				if len(rule.Do.pre) == 0 {
					executedPre[i] = true
				}
				continue
			}

			executedPre[i] = true
			if err := execPreCommand(rule.Do, rm, r); err != nil {
				if errors.Is(err, errTerminateRule) {
					terminatedInPre[i] = true
					preTerminated = true
					continue
				}
				if isUnexpectedError(err) {
					// will logged by logFlushError after FlushRelease
					rm.AppendError("executing pre rule (%s): %w", rule.Do.raw, err)
				}
				hasError = true
			}
		}

		// Default rule is a fallback: run only when no non-default pre rule matched.
		defaultExecutedPre := false
		defaultTerminatedInPre := false
		if defaultRule != nil && !matchedNonDefaultPre && !defaultRule.On.phase.IsPostRule() && defaultRule.On.Check(rm, r) {
			defaultExecutedPre = true
			if err := execPreCommand(defaultRule.Do, rm, r); err != nil {
				if errors.Is(err, errTerminateRule) {
					defaultTerminatedInPre = true
				} else {
					if isUnexpectedError(err) {
						// will logged by logFlushError after FlushRelease
						rm.AppendError("executing pre rule (%s): %w", defaultRule.Do.raw, err)
					}
					hasError = true
				}
			}
		}

		if !rm.HasStatus() {
			if hasError {
				http.Error(rm, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
			} else { // call upstream if no WriteHeader or Write was called and no error occurred
				up(rm, r)
			}
		}

		// Run post commands for rules that actually executed in pre phase,
		// unless that same rule terminated in pre phase.
		for i, rule := range nonDefaultRules {
			if !executedPre[i] || terminatedInPre[i] {
				continue
			}
			if err := execPostCommand(rule.Do, rm, r); err != nil {
				if errors.Is(err, errTerminateRule) {
					continue
				}
				if isUnexpectedError(err) {
					// will logged by logFlushError after FlushRelease
					rm.AppendError("executing post rule (%s): %w", rule.Do.raw, err)
				}
			}
		}
		if defaultExecutedPre && !defaultTerminatedInPre {
			if err := execPostCommand(defaultRule.Do, rm, r); err != nil {
				if !errors.Is(err, errTerminateRule) && isUnexpectedError(err) {
					// will logged by logFlushError after FlushRelease
					rm.AppendError("executing post rule (%s): %w", defaultRule.Do.raw, err)
				}
			}
		}

		// Run true post-matcher rules after response is available.
		for _, rule := range nonDefaultRules {
			if !rule.On.phase.IsPostRule() || !rule.On.Check(rm, r) {
				continue
			}
			// Post-rule matchers are only evaluated after upstream, so commands parsed
			// as "pre" for requirement purposes still need to run in this phase.
			if err := rule.Do.pre.ServeHTTP(rm, r, up); err != nil {
				if errors.Is(err, errTerminateRule) {
					continue
				}
				if isUnexpectedError(err) {
					// will logged by logFlushError after FlushRelease
					rm.AppendError("executing pre rule (%s): %w", rule.Do.raw, err)
				}
			}
			if err := execPostCommand(rule.Do, rm, r); err != nil {
				if errors.Is(err, errTerminateRule) {
					continue
				}
				if isUnexpectedError(err) {
					// will logged by logFlushError after FlushRelease
					rm.AppendError("executing post rule (%s): %w", rule.Do.raw, err)
				}
			}
		}
	}
}

func (rule *Rule) String() string {
	return rule.Name
}

func (rule *Rule) Check(w *httputils.ResponseModifier, r *http.Request) bool {
	if rule.On.checker == nil {
		return true
	}
	return rule.On.Check(w, r)
}

//go:linkname errStreamClosed golang.org/x/net/http2.errStreamClosed
var errStreamClosed error

//go:linkname errClientDisconnected golang.org/x/net/http2.errClientDisconnected
var errClientDisconnected error

func isUnexpectedError(err error) bool {
	if errors.Is(err, errStreamClosed) || errors.Is(err, errClientDisconnected) {
		return false
	}
	if h2Err, ok := errors.AsType[http2.StreamError](err); ok {
		// ignore these errors
		if h2Err.Code == http2.ErrCodeStreamClosed {
			return false
		}
	}
	if h3Err, ok := errors.AsType[*http3.Error](err); ok {
		// ignore these errors
		switch h3Err.ErrorCode {
		case
			http3.ErrCodeNoError,
			http3.ErrCodeRequestCanceled:
			return false
		}
	}
	return true
}

func logFlushError(err error, r *http.Request) {
	log.Err(err).Str("method", r.Method).Str("url", r.Host+r.URL.Path).Msg("error executing rules")
}
