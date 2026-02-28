package middleware

import (
	"net/http"

	"github.com/rs/zerolog/log"
	"github.com/yusing/godoxy/internal/route/rules"
	httputils "github.com/yusing/goutils/http"
)

type Bypass []rules.RuleOn

type (
	checkReqFunc  func(r *http.Request) bool
	checkRespFunc func(resp *http.Response) bool
)

func (b Bypass) ShouldBypass(w http.ResponseWriter, r *http.Request) bool {
	for _, rule := range b {
		if rule.Check(w, r) {
			log.Debug().Str("rule_matched", rule.String()).Str("url", r.Host+r.URL.Path).Msg("bypassing request")
			return true
		}
	}
	return false
}

type checkBypass struct {
	name string

	bypass Bypass
	modReq RequestModifier
	modRes ResponseModifier

	modReqCheckEnforceFuncs []checkReqFunc
	modReqCheckBypassFuncs  []checkReqFunc

	modResCheckEnforceFuncs []checkRespFunc
	modResCheckBypassFuncs  []checkRespFunc
}

var (
	_ RequestModifier  = (*checkBypass)(nil)
	_ ResponseModifier = (*checkBypass)(nil)
)

// shouldModReqEnforce checks if the modify request should be enforced.
//
// Returns true if any of the check functions returns true.
func (c *checkBypass) shouldModReqEnforce(r *http.Request) bool {
	for _, f := range c.modReqCheckEnforceFuncs {
		if f(r) {
			return true
		}
	}
	return false
}

// shouldModResEnforce checks if the modify response should be enforced.
//
// Returns true if any of the check functions returns true.
func (c *checkBypass) shouldModResEnforce(resp *http.Response) bool {
	for _, f := range c.modResCheckEnforceFuncs {
		if f(resp) {
			return true
		}
	}
	return false
}

// shouldModReqBypass checks if the modify request should be bypassed.
//
// If enforce checks return true, the bypass checks are not performed.
// Otherwise, if any of the bypass checks returns true
// or user defined bypass rules return true, the request is bypassed.
func (c *checkBypass) shouldModReqBypass(w http.ResponseWriter, r *http.Request) bool {
	if c.shouldModReqEnforce(r) {
		return false
	}
	for _, f := range c.modReqCheckBypassFuncs {
		if f(r) {
			return true
		}
	}
	return c.bypass.ShouldBypass(w, r)
}

// shouldModResBypass checks if the modify response should be bypassed.
//
// If enforce checks return true, the bypass checks are not performed.
// Otherwise, if any of the bypass checks returns true
// or user defined bypass rules return true, the response is bypassed.
func (c *checkBypass) shouldModResBypass(resp *http.Response) bool {
	if c.shouldModResEnforce(resp) {
		return false
	}
	for _, f := range c.modResCheckBypassFuncs {
		if f(resp) {
			return true
		}
	}
	return c.bypass.ShouldBypass(httputils.ResponseAsRW(resp), resp.Request)
}

// before modifies the request if the request should be modified.
//
// Returns true if the request is not done, false otherwise.
func (c *checkBypass) before(w http.ResponseWriter, r *http.Request) (proceedNext bool) {
	if c.modReq == nil || c.shouldModReqBypass(w, r) {
		return true
	}
	// log.Debug().Str("middleware", c.name).Str("url", r.Host+r.URL.Path).Msg("modifying request")
	return c.modReq.before(w, r)
}

// modifyResponse modifies the response if the response should be modified.
func (c *checkBypass) modifyResponse(resp *http.Response) error {
	if c.modRes == nil || c.shouldModResBypass(resp) {
		return nil
	}
	// log.Debug().Str("middleware", c.name).Str("url", resp.Request.Host+resp.Request.URL.Path).Msg("modifying response")
	return c.modRes.modifyResponse(resp)
}

func (m *Middleware) withCheckBypass() any {
	if len(m.Bypass) > 0 {
		modReq, _ := m.impl.(RequestModifier)
		modRes, _ := m.impl.(ResponseModifier)
		return &checkBypass{
			name:                    m.Name(),
			bypass:                  m.Bypass,
			modReq:                  modReq,
			modRes:                  modRes,
			modReqCheckEnforceFuncs: getModReqCheckEnforceFuncs(modReq),
			modReqCheckBypassFuncs:  getModReqCheckBypassFuncs(modReq),
			modResCheckEnforceFuncs: getModResCheckEnforceFuncs(modRes),
			modResCheckBypassFuncs:  getModResCheckBypassFuncs(modRes),
		}
	}
	return m.impl
}

func getModReqCheckEnforceFuncs(modReq RequestModifier) (checks []checkReqFunc) {
	if modReq == nil {
		return nil
	}
	if _, ok := modReq.(*oidcMiddleware); ok {
		checks = append(checks, isOIDCAuthPath)
	}
	return checks
}

func getModReqCheckBypassFuncs(modReq RequestModifier) (checks []checkReqFunc) {
	if modReq == nil {
		return nil
	}
	switch modReq.(type) {
	case *oidcMiddleware, *forwardAuthMiddleware, *crowdsecMiddleware, *hCaptcha:
		checks = append(checks, isStaticAssetPath)
	}
	return checks
}

func getModResCheckEnforceFuncs(modRes ResponseModifier) []checkRespFunc {
	// TODO: add enforce checks for response modifiers if needed.
	return nil
}

func getModResCheckBypassFuncs(modRes ResponseModifier) []checkRespFunc {
	// TODO: add bypass checks for response modifiers if needed.
	return nil
}
