package rules

import (
	"errors"
	"fmt"
	"net/http"

	"github.com/bytedance/sonic"
	"github.com/rs/zerolog/log"
)

type (
	/*
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
	Rules []Rule
	/*
		Rule is a rule for a reverse proxy.
		It do `Do` when `On` matches.

		A rule can have multiple lines of on.

		All lines of on must match,
		but each line can have multiple checks that
		one match means this line is matched.
	*/
	Rule struct {
		Name string  `json:"name"`
		On   RuleOn  `json:"on" swaggertype:"string"`
		Do   Command `json:"do" swaggertype:"string"`
	}
)

func (rule *Rule) IsResponseRule() bool {
	return rule.On.IsResponseChecker() || rule.Do.IsResponseHandler()
}

// BuildHandler returns a http.HandlerFunc that implements the rules.
func (rules Rules) BuildHandler(up http.HandlerFunc) http.HandlerFunc {
	if len(rules) == 0 {
		return up
	}

	defaultRule := Rule{
		Name: "default",
		Do: Command{
			raw:  "pass",
			exec: BypassCommand{},
		},
	}

	var nonDefaultRules Rules
	hasDefaultRule := false
	for i, rule := range rules {
		if rule.Name == "default" {
			defaultRule = rule
			hasDefaultRule = true
		} else {
			// set name to index if name is empty
			if rule.Name == "" {
				rule.Name = fmt.Sprintf("rule[%d]", i)
			}
			nonDefaultRules = append(nonDefaultRules, rule)
		}
	}

	if len(nonDefaultRules) == 0 {
		if defaultRule.Do.isBypass() {
			return up
		}
		if defaultRule.IsResponseRule() {
			return func(w http.ResponseWriter, r *http.Request) {
				rm := NewResponseModifier(w)
				defer func() {
					if _, err := rm.FlushRelease(); err != nil {
						logError(err, r)
					}
				}()
				w = rm
				up(w, r)
				err := defaultRule.Do.exec.Handle(w, r)
				if err != nil && !errors.Is(err, errTerminated) {
					rm.AppendError(defaultRule, err)
				}
			}
		}
		return func(w http.ResponseWriter, r *http.Request) {
			rm := NewResponseModifier(w)
			defer func() {
				if _, err := rm.FlushRelease(); err != nil {
					logError(err, r)
				}
			}()
			w = rm
			err := defaultRule.Do.exec.Handle(w, r)
			if err == nil {
				up(w, r)
				return
			}
			if !errors.Is(err, errTerminated) {
				rm.AppendError(defaultRule, err)
			}
		}
	}

	preRules := make(Rules, 0, len(nonDefaultRules))
	postRules := make(Rules, 0, len(nonDefaultRules))
	for _, rule := range nonDefaultRules {
		if rule.IsResponseRule() {
			postRules = append(postRules, rule)
		} else {
			preRules = append(preRules, rule)
		}
	}

	isDefaultRulePost := hasDefaultRule && defaultRule.IsResponseRule()
	defaultTerminates := isTerminatingHandler(defaultRule.Do.exec)

	return func(w http.ResponseWriter, r *http.Request) {
		rm := NewResponseModifier(w)
		defer func() {
			if _, err := rm.FlushRelease(); err != nil {
				logError(err, r)
			}
		}()

		w = rm

		shouldCallUpstream := true
		preMatched := false

		if hasDefaultRule && !isDefaultRulePost && !defaultTerminates {
			if defaultRule.Do.isBypass() {
				// continue to upstream
			} else {
				err := defaultRule.Handle(w, r)
				if err != nil {
					if !errors.Is(err, errTerminated) {
						rm.AppendError(defaultRule, err)
					}
					shouldCallUpstream = false
				}
			}
		}

		if shouldCallUpstream {
			for _, rule := range preRules {
				if rule.Check(w, r) {
					preMatched = true
					if rule.Do.isBypass() {
						break // post rules should still execute
					}
					err := rule.Handle(w, r)
					if err != nil {
						if !errors.Is(err, errTerminated) {
							rm.AppendError(rule, err)
						}
						shouldCallUpstream = false
						break
					}
				}
			}
		}

		if hasDefaultRule && !isDefaultRulePost && defaultTerminates && shouldCallUpstream && !preMatched {
			if defaultRule.Do.isBypass() {
				// continue to upstream
			} else {
				err := defaultRule.Handle(w, r)
				if err != nil {
					if !errors.Is(err, errTerminated) {
						rm.AppendError(defaultRule, err)
						return
					}
					shouldCallUpstream = false
				}
			}
		}

		if shouldCallUpstream {
			up(w, r)
		}

		// if no post rules, we are done here
		if len(postRules) == 0 && !isDefaultRulePost {
			return
		}

		for _, rule := range postRules {
			if rule.Check(w, r) {
				err := rule.Handle(w, r)
				if err != nil {
					if !errors.Is(err, errTerminated) {
						rm.AppendError(rule, err)
					}
					return
				}
			}
		}

		if isDefaultRulePost {
			err := defaultRule.Handle(w, r)
			if err != nil && !errors.Is(err, errTerminated) {
				rm.AppendError(defaultRule, err)
			}
		}
	}
}

func isTerminatingHandler(handler CommandHandler) bool {
	switch h := handler.(type) {
	case TerminatingCommand:
		return true
	case Commands:
		if len(h) == 0 {
			return false
		}
		return isTerminatingHandler(h[len(h)-1])
	default:
		return false
	}
}

func (rules Rules) MarshalJSON() ([]byte, error) {
	names := make([]string, len(rules))
	for i, rule := range rules {
		names[i] = rule.Name
	}
	return sonic.Marshal(names)
}

func (rule *Rule) String() string {
	return rule.Name
}

func (rule *Rule) Check(w http.ResponseWriter, r *http.Request) bool {
	if rule.On.checker == nil {
		return true
	}
	v := rule.On.checker.Check(w, r)
	return v
}

func (rule *Rule) Handle(w http.ResponseWriter, r *http.Request) error {
	return rule.Do.exec.Handle(w, r)
}

func logError(err error, r *http.Request) {
	log.Err(err).Str("method", r.Method).Str("url", r.Host+r.URL.Path).Msg("error executing rules")
}
