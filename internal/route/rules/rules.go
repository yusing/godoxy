package rules

import (
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
//
//	if a bypass rule matches,
//	the request is passed to the upstream and no more rules are executed.
//
//	if no rule matches, the default rule is executed
//	if no rule matches and default rule is not set,
//	the request is passed to the upstream.
func (rules Rules) BuildHandler(up http.Handler) http.HandlerFunc {
	defaultRule := Rule{
		Name: "default",
		Do: Command{
			raw:  "pass",
			exec: BypassCommand{},
		},
	}

	var nonDefaultRules Rules
	hasDefaultRule := false
	hasResponseRule := false
	modifyResponse := false
	for _, rule := range rules {
		if rule.Name == "default" {
			defaultRule = rule
			hasDefaultRule = true
		} else {
			if rule.IsResponseRule() {
				hasResponseRule = true
			} else if hasResponseRule {
				origUp := up
				up = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					w = GetInitResponseModifier(w)
					origUp.ServeHTTP(w, r)
					// TODO: cache should be shared
					cache := NewCache()
					defer cache.Release()
					if rule.Check(cache, r) {
						rule.Do.exec.Handle(cache, w, r)
					}
				})
				modifyResponse = true
			} else {
				nonDefaultRules = append(nonDefaultRules, rule)
			}
		}
	}

	if modifyResponse {
		up = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			rm := GetInitResponseModifier(w)
			up.ServeHTTP(rm, r)
			if _, err := rm.FlushRelease(); err != nil {
				log.Err(err).Msg("failed to flush response modifier")
			}
		})
	}

	if len(rules) == 0 {
		if defaultRule.Do.isBypass() {
			return up.ServeHTTP
		}
		if defaultRule.IsResponseRule() {
			return func(w http.ResponseWriter, r *http.Request) {
				cache := NewCache()
				defer cache.Release()
				up.ServeHTTP(w, r)
				defaultRule.Do.exec.Handle(cache, w, r)
			}
		}
		return func(w http.ResponseWriter, r *http.Request) {
			cache := NewCache()
			defer cache.Release()
			if defaultRule.Do.exec.Handle(cache, w, r) {
				up.ServeHTTP(w, r)
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

	return func(w http.ResponseWriter, r *http.Request) {
		cache := NewCache()
		defer cache.Release()

		for _, rule := range preRules {
			if rule.Check(cache, r) {
				if rule.Do.isBypass() {
					up.ServeHTTP(w, r)
					break
				}
				if !rule.Handle(cache, w, r) {
					break
				}
			}
		}

		if hasDefaultRule && !isDefaultRulePost {
			if defaultRule.Do.isBypass() {
				// continue to upstream
			} else if !defaultRule.Handle(cache, w, r) {
				return
			}
		}

		// if no post rules, we are done here
		if len(postRules) == 0 && !isDefaultRulePost {
			up.ServeHTTP(w, r)
			return
		}

		for _, rule := range postRules {
			if rule.Check(cache, r) {
				// for post-request rules, proceed=false means stop processing more post-rules
				// for now it always proceed
				if !rule.Handle(cache, w, r) {
					return
				}
			}
		}

		if hasDefaultRule && isDefaultRulePost {
			defaultRule.Handle(cache, w, r)
		}
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

func (rule *Rule) Check(cached Cache, r *http.Request) bool {
	return rule.On.checker.Check(cached, r)
}

func (rule *Rule) Handle(cached Cache, w http.ResponseWriter, r *http.Request) (proceed bool) {
	proceed = rule.Do.exec.Handle(cached, w, r)
	return proceed
}
