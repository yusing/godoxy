package middleware

import (
	"net/http"
	"strings"

	"github.com/yusing/godoxy/internal/auth"
	"github.com/yusing/godoxy/internal/route/rules"
)

type Bypass []rules.RuleOn

func (b Bypass) ShouldBypass(w http.ResponseWriter, r *http.Request) bool {
	for _, rule := range b {
		if rule.Check(w, r) {
			return true
		}
	}
	return false
}

type checkBypass struct {
	bypass Bypass
	modReq RequestModifier
	modRes ResponseModifier

	// when request path matches any of these prefixes, bypass is not applied
	enforcedPathPrefixes []string
}

func (c *checkBypass) isEnforced(r *http.Request) bool {
	for _, prefix := range c.enforcedPathPrefixes {
		if strings.HasPrefix(r.URL.Path, prefix) {
			return true
		}
	}
	return false
}

func (c *checkBypass) before(w http.ResponseWriter, r *http.Request) (proceedNext bool) {
	if c.modReq == nil || (!c.isEnforced(r) && c.bypass.ShouldBypass(w, r)) {
		return true
	}
	return c.modReq.before(w, r)
}

func (c *checkBypass) modifyResponse(resp *http.Response) error {
	if c.modRes == nil || (!c.isEnforced(resp.Request) && c.bypass.ShouldBypass(rules.ResponseAsRW(resp), resp.Request)) {
		return nil
	}
	return c.modRes.modifyResponse(resp)
}

func (m *Middleware) withCheckBypass() any {
	if len(m.Bypass) > 0 {
		modReq, _ := m.impl.(RequestModifier)
		modRes, _ := m.impl.(ResponseModifier)
		return &checkBypass{
			bypass:               m.Bypass,
			enforcedPathPrefixes: getEnforcedPathPrefixes(modReq, modRes),
			modReq:               modReq,
			modRes:               modRes,
		}
	}
	return m.impl
}

func getEnforcedPathPrefixes(modReq RequestModifier, modRes ResponseModifier) []string {
	if modReq == nil && modRes == nil {
		return nil
	}
	switch modReq.(type) {
	case *oidcMiddleware:
		return []string{auth.OIDCAuthBasePath}
	}
	return nil
}
