package middleware

import (
	"errors"
	"fmt"
	"io/fs"
	"path"

	"github.com/rs/zerolog/log"
	"github.com/yusing/godoxy/internal/common"
	"github.com/yusing/godoxy/internal/route/rules"
	gperr "github.com/yusing/goutils/errs"
	fsutils "github.com/yusing/goutils/fs"
	strutils "github.com/yusing/goutils/strings"
)

// snakes and cases will be stripped on `Get`
// so keys are lowercase without snake.
var allMiddlewares = map[string]*Middleware{
	"redirecthttp": RedirectHTTP,

	"oidc":        OIDC,
	"forwardauth": ForwardAuth,
	"crowdsec":    Crowdsec,

	"request":        ModifyRequest,
	"modifyrequest":  ModifyRequest,
	"response":       ModifyResponse,
	"modifyresponse": ModifyResponse,
	"setxforwarded":  SetXForwarded,
	"hidexforwarded": HideXForwarded,

	"modifyhtml": ModifyHTML,
	"themed":     Themed,

	"errorpage":       CustomErrorPage,
	"customerrorpage": CustomErrorPage,

	"realip":           RealIP,
	"cloudflarerealip": CloudflareRealIP,

	"cidrwhitelist": CIDRWhiteList,
	"ratelimit":     RateLimiter,

	"hcaptcha": HCaptcha,
}

var (
	ErrUnknownMiddleware       = errors.New("unknown middleware")
	ErrMiddlewareAlreadyExists = errors.New("middleware with the same name already exists")
)

func init() {
	rules.InitRequestMiddlewareResolver(func(name string, options map[string]any) (rules.RequestMiddleware, error) {
		definition, err := Get(name)
		if err != nil {
			return nil, err
		}

		middleware, err := definition.New(OptionsRaw(options))
		if err != nil {
			return nil, err
		}

		if !hasRequestPhase(middleware.impl) {
			return nil, fmt.Errorf("middleware %q has no request phase", name)
		}

		return middleware.TryModifyRequest, nil
	})
}

func hasRequestPhase(impl any) bool {
	switch impl := impl.(type) {
	case *checkBypass:
		return impl.modReq != nil && hasRequestPhase(impl.modReq)
	case *middlewareChain:
		for _, before := range impl.befores {
			if hasRequestPhase(before) {
				return true
			}
		}
		return false
	case RequestModifier:
		return true
	default:
		return false
	}
}

func Get(name string) (*Middleware, error) {
	middleware, ok := allMiddlewares[strutils.ToLowerNoSnake(name)]
	if !ok {
		return nil, gperr.PrependSubject(ErrUnknownMiddleware, name).
			With(gperr.DoYouMeanField(name, allMiddlewares))
	}
	return middleware, nil
}

func LoadComposeFiles() {
	var errs gperr.Builder
	middlewareDefs, err := fsutils.ListFiles(common.MiddlewareComposeBasePath, 0)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return
		}
		log.Err(err).Msg("failed to list middleware definitions")
		return
	}
	for _, defFile := range middlewareDefs {
		voidErrs := gperr.NewBuilder("") // ignore these errors, will be added in next step
		mws := BuildMiddlewaresFromComposeFile(defFile, &voidErrs)
		if len(mws) == 0 {
			continue
		}
		for name, m := range mws {
			name = strutils.ToLowerNoSnake(name)
			if _, ok := allMiddlewares[name]; ok {
				errs.AddSubject(ErrMiddlewareAlreadyExists, name)
				continue
			}
			allMiddlewares[name] = m
			log.Info().
				Str("src", path.Base(defFile)).
				Str("name", name).
				Msg("middleware loaded")
		}
	}
	// build again to resolve cross references
	for _, defFile := range middlewareDefs {
		mws := BuildMiddlewaresFromComposeFile(defFile, &errs)
		if len(mws) == 0 {
			continue
		}
		for name, m := range mws {
			name = strutils.ToLowerNoSnake(name)
			if _, ok := allMiddlewares[name]; ok {
				// already loaded above
				continue
			}
			allMiddlewares[name] = m
			log.Info().
				Str("src", path.Base(defFile)).
				Str("name", name).
				Msg("middleware loaded")
		}
	}
	if errs.HasError() {
		log.Err(errs.Error()).Msg("middleware compile errors")
	}
}
