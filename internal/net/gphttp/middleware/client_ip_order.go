package middleware

import (
	"fmt"
)

// clientIPRole classifies a middleware by how it relates to the client IP.
type clientIPRole int

const (
	clientIPRoleNone clientIPRole = iota
	// clientIPRoleResolver rewrites the request's client IP (real IP).
	clientIPRoleResolver
	// clientIPRoleReader behaves differently depending on the client IP.
	clientIPRoleReader
)

// implRole returns the role of a middleware implementation. A compose chain
// takes the role of its first member that has one: a chain that resolves the
// client IP before reading it acts as a resolver. A member whose bypass rules
// match `remote` reads the client IP even if the middleware itself does not.
func implRole(impl any) clientIPRole {
	if bypass, ok := impl.(*checkBypass); ok {
		role := implRole(bypass.modReq)
		if role == clientIPRoleNone && bypass.modReq != nil && bypass.bypass.usesRemote() {
			return clientIPRoleReader
		}
		return role
	}
	switch impl := impl.(type) {
	case *realIP, *cloudflareRealIP:
		return clientIPRoleResolver
	case *cidrWhitelist, *rateLimiter, *crowdsecMiddleware, *forwardAuthMiddleware, *setXForwarded:
		return clientIPRoleReader
	case *middlewareChain:
		for _, before := range impl.befores {
			if role := implRole(before); role != clientIPRoleNone {
				return role
			}
		}
	}
	return clientIPRoleNone
}

// clientIPFacts keeps reading and resolving independent for diagnostics. Unlike
// the first-role classification used for sorting, a compose chain can do both.
type clientIPFacts struct {
	reads    bool
	resolves bool
	request  bool
}

func implClientIPFacts(impl any) clientIPFacts {
	switch impl := impl.(type) {
	case *checkBypass:
		// Response-only bypasses run after the request chain has finished.
		if impl.modReq == nil {
			return clientIPFacts{}
		}
		facts := implClientIPFacts(impl.modReq)
		facts.reads = facts.reads || impl.bypass.usesRemote()
		return facts
	case *middlewareChain:
		facts := clientIPFacts{request: true}
		for _, before := range impl.befores {
			child := implClientIPFacts(before)
			facts.reads = facts.reads || child.reads
			facts.resolves = facts.resolves || child.resolves
		}
		return facts
	default:
		role := implRole(impl)
		_, request := impl.(RequestModifier)
		return clientIPFacts{reads: role == clientIPRoleReader, resolves: role == clientIPRoleResolver, request: request}
	}
}

func defClientIPFacts(def map[string]any) clientIPFacts {
	use, _ := def["use"].(string)
	m, err := Get(use)
	if err != nil {
		return clientIPFacts{}
	}
	if m.construct != nil {
		// built-in: classify a zero instance of its implementation
		return implClientIPFacts(m.construct())
	}
	return implClientIPFacts(m.impl)
}

// ClientIPOrderWarnings checks a list-form middleware chain (entrypoint or
// compose file), where middlewares run in declared order. It returns one
// warning per middleware that reads the client IP, directly, through a
// compose chain, or through a `remote` bypass rule, but runs before the
// chain's last real IP middleware. Such a middleware sees an address that is
// rewritten later, e.g. cloudflared's 127.0.0.1 passing a cidr_whitelist that
// allows loopback.
//
// Chains without a real IP middleware produce no warnings.
func ClientIPOrderWarnings(name string, defs []map[string]any) []string {
	facts := make([]clientIPFacts, len(defs))
	lastResolver := -1
	for i, def := range defs {
		facts[i] = defClientIPFacts(def)
		if facts[i].resolves {
			lastResolver = i
		}
	}
	if lastResolver <= 0 {
		return nil
	}
	resolver, _ := defs[lastResolver]["use"].(string)

	var warnings []string
	for i, def := range defs[:lastResolver] {
		use, _ := def["use"].(string)
		var reason string
		switch {
		case facts[i].reads:
			reason = "reads the client IP"
		case facts[i].request && bypassUsesRemote(def["bypass"]):
			reason = "has bypass rules matching `remote`"
		default:
			continue
		}
		warnings = append(warnings, fmt.Sprintf(
			"%s[%d] %s %s but runs before %s[%d] %s, so it sees the upstream proxy's address instead of the client's; move %s before it",
			name, i, use, reason, name, lastResolver, resolver, resolver,
		))
	}
	return warnings
}

func bypassUsesRemote(raw any) bool {
	bypass, ok, err := parseBypassValue(raw)
	if err != nil || !ok {
		return false
	}
	return bypass.usesRemote()
}

// usesRemote reports whether any rule matches on the client IP (`remote`).
func (b Bypass) usesRemote() bool {
	for _, rule := range b {
		if rule.UsesClientIP() {
			return true
		}
	}
	return false
}
