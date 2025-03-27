package middleware

import (
	"path"

	"github.com/yusing/go-proxy/internal/common"
	"github.com/yusing/go-proxy/internal/gperr"
	"github.com/yusing/go-proxy/internal/logging"
	"github.com/yusing/go-proxy/internal/utils"
	"github.com/yusing/go-proxy/internal/utils/strutils"
)

// snakes and cases will be stripped on `Get`
// so keys are lowercase without snake.
var allMiddlewares = map[string]*Middleware{
	"redirecthttp": RedirectHTTP,

	"oidc": OIDC,

	"request":        ModifyRequest,
	"modifyrequest":  ModifyRequest,
	"response":       ModifyResponse,
	"modifyresponse": ModifyResponse,
	"setxforwarded":  SetXForwarded,
	"hidexforwarded": HideXForwarded,

	"errorpage":       CustomErrorPage,
	"customerrorpage": CustomErrorPage,

	"realip":           RealIP,
	"cloudflarerealip": CloudflareRealIP,

	"cidrwhitelist": CIDRWhiteList,
	"ratelimit":     RateLimiter,
}

var (
	ErrUnknownMiddleware    = gperr.New("unknown middleware")
	ErrDuplicatedMiddleware = gperr.New("duplicated middleware")
)

func Get(name string) (*Middleware, Error) {
	middleware, ok := allMiddlewares[strutils.ToLowerNoSnake(name)]
	if !ok {
		return nil, ErrUnknownMiddleware.
			Subject(name).
			Withf(strutils.DoYouMean(utils.NearestField(name, allMiddlewares)))
	}
	return middleware, nil
}

func All() map[string]*Middleware {
	return allMiddlewares
}

func LoadComposeFiles() {
	errs := gperr.NewBuilder("middleware compile errors")
	middlewareDefs, err := utils.ListFiles(common.MiddlewareComposeBasePath, 0)
	if err != nil {
		logging.Err(err).Msg("failed to list middleware definitions")
		return
	}
	for _, defFile := range middlewareDefs {
		voidErrs := gperr.NewBuilder("") // ignore these errors, will be added in next step
		mws := BuildMiddlewaresFromComposeFile(defFile, voidErrs)
		if len(mws) == 0 {
			continue
		}
		for name, m := range mws {
			name = strutils.ToLowerNoSnake(name)
			if _, ok := allMiddlewares[name]; ok {
				errs.Add(ErrDuplicatedMiddleware.Subject(name))
				continue
			}
			allMiddlewares[name] = m
			logging.Info().
				Str("src", path.Base(defFile)).
				Str("name", name).
				Msg("middleware loaded")
		}
	}
	// build again to resolve cross references
	for _, defFile := range middlewareDefs {
		mws := BuildMiddlewaresFromComposeFile(defFile, errs)
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
			logging.Info().
				Str("src", path.Base(defFile)).
				Str("name", name).
				Msg("middleware loaded")
		}
	}
	if errs.HasError() {
		gperr.LogError(errs.About(), errs.Error())
	}
}
