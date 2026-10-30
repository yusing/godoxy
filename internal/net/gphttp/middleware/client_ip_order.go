package middleware

import (
	"fmt"
	"regexp"

	strutils "github.com/yusing/goutils/strings"
)

// Middlewares (normalized names) that rewrite the request's client IP.
var clientIPResolverNames = map[string]struct{}{
	"realip":           {},
	"cloudflarerealip": {},
}

// Middlewares (normalized names) whose behavior depends on the client IP.
var clientIPReaderNames = map[string]struct{}{
	"cidrwhitelist": {},
	"ratelimit":     {},
	"crowdsec":      {},
	"forwardauth":   {},
	"setxforwarded": {},
}

var remoteMatcherRe = regexp.MustCompile(`(^|[\s&|!(])remote(\s|$)`)

// ClientIPOrderWarnings checks a list-form middleware chain (entrypoint or
// compose file), where middlewares run in declared order. It returns one
// warning per middleware that reads the client IP, directly or through a
// `remote` bypass rule, but runs before the chain's first real IP middleware.
// Such a middleware sees the proxy's address instead of the client's, e.g.
// cloudflared on 127.0.0.1 passing a cidr_whitelist that allows loopback.
//
// Chains without a real IP middleware produce no warnings.
func ClientIPOrderWarnings(name string, defs []map[string]any) []string {
	resolverIdx := -1
	for i, def := range defs {
		if _, ok := clientIPResolverNames[defUse(def)]; ok {
			resolverIdx = i
			break
		}
	}
	if resolverIdx <= 0 {
		return nil
	}
	resolver, _ := defs[resolverIdx]["use"].(string)

	var warnings []string
	for i, def := range defs[:resolverIdx] {
		use, _ := def["use"].(string)
		var reason string
		if _, ok := clientIPReaderNames[defUse(def)]; ok {
			reason = "reads the client IP"
		} else if bypassUsesRemote(def["bypass"]) {
			reason = "has bypass rules matching `remote`"
		} else {
			continue
		}
		warnings = append(warnings, fmt.Sprintf(
			"%s[%d] %s %s but runs before %s[%d] %s, so it sees the upstream proxy's address instead of the client's; move %s before it",
			name, i, use, reason, name, resolverIdx, resolver, resolver,
		))
	}
	return warnings
}

func defUse(def map[string]any) string {
	use, _ := def["use"].(string)
	return strutils.ToLowerNoSnake(use)
}

func bypassUsesRemote(raw any) bool {
	bypass, ok, err := parseBypassValue(raw)
	if err != nil || !ok {
		return false
	}
	for _, rule := range bypass {
		if remoteMatcherRe.MatchString(rule.String()) {
			return true
		}
	}
	return false
}
