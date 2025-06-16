package rules

import (
	"encoding/json"
	"net/http"
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
	Rules []*Rule
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
		On   RuleOn  `json:"on"`
		Do   Command `json:"do"`
	}
)

// BuildHandler returns a http.HandlerFunc that implements the rules.
//
//	if a bypass rule matches,
//	the request is passed to the upstream and no more rules are executed.
//
//	if no rule matches, the default rule is executed
//	if no rule matches and default rule is not set,
//	the request is passed to the upstream.
func (rules Rules) BuildHandler(caller string, up http.Handler) http.HandlerFunc {
	defaultRule := &Rule{
		Name: "default",
		Do: Command{
			raw:  "pass",
			exec: BypassCommand{},
		},
	}

	nonDefaultRules := make(Rules, 0, len(rules))
	for i, rule := range rules {
		if rule.Name == "default" {
			defaultRule = rule
			nonDefaultRules = append(nonDefaultRules, rules[:i]...)
			nonDefaultRules = append(nonDefaultRules, rules[i+1:]...)
			break
		}
	}

	if len(rules) == 0 {
		if defaultRule.Do.isBypass() {
			return up.ServeHTTP
		}
		return func(w http.ResponseWriter, r *http.Request) {
			cache := NewCache()
			defer cache.Release()
			if defaultRule.Do.exec.Handle(cache, w, r) {
				up.ServeHTTP(w, r)
			}
		}
	}

	if len(nonDefaultRules) == 0 {
		nonDefaultRules = rules
	}

	return func(w http.ResponseWriter, r *http.Request) {
		cache := NewCache()
		defer cache.Release()

		for _, rule := range nonDefaultRules {
			if rule.Check(cache, r) {
				if rule.Do.isBypass() {
					up.ServeHTTP(w, r)
					return
				}
				if !rule.Handle(cache, w, r) {
					return
				}
			}
		}

		// bypass or proceed
		if defaultRule.Do.isBypass() || defaultRule.Handle(cache, w, r) {
			up.ServeHTTP(w, r)
		}
	}
}

func (rules Rules) MarshalJSON() ([]byte, error) {
	names := make([]string, len(rules))
	for i, rule := range rules {
		names[i] = rule.Name
	}
	return json.Marshal(names)
}

func (rule *Rule) String() string {
	return rule.Name
}

func (rule *Rule) Check(cached Cache, r *http.Request) bool {
	return rule.On.checker.Check(cached, r)
}

func (rule *Rule) Handle(cached Cache, w http.ResponseWriter, r *http.Request) (proceed bool) {
	proceed = rule.Do.exec.Handle(cached, w, r)
	return
}
