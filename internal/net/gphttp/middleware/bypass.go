package middleware

import (
	"net/http"

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
}

func (c *checkBypass) before(w http.ResponseWriter, r *http.Request) (proceedNext bool) {
	if c.modReq == nil || c.bypass.ShouldBypass(w, r) {
		return true
	}
	return c.modReq.before(w, r)
}

func (c *checkBypass) modifyResponse(resp *http.Response) error {
	if c.modRes == nil || c.bypass.ShouldBypass(rules.ResponseAsRW(resp), resp.Request) {
		return nil
	}
	return c.modRes.modifyResponse(resp)
}

func (m *Middleware) withCheckBypass() any {
	if len(m.Bypass) > 0 {
		modReq, _ := m.impl.(RequestModifier)
		modRes, _ := m.impl.(ResponseModifier)
		return &checkBypass{
			bypass: m.Bypass,
			modReq: modReq,
			modRes: modRes,
		}
	}
	return m.impl
}
