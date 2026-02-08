package middleware

import (
	"errors"
	"io/fs"
	"path"

	"github.com/rs/zerolog/log"
	"github.com/yusing/godoxy/internal/common"
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

func Get(name string) (*Middleware, error) {
	middleware, ok := allMiddlewares[strutils.ToLowerNoSnake(name)]
	if !ok {
		return nil, gperr.PrependSubject(ErrUnknownMiddleware, name).
			With(gperr.DoYouMeanField(name, allMiddlewares))
	}
	return middleware, nil
}

func All() map[string]*Middleware {
	return allMiddlewares
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
