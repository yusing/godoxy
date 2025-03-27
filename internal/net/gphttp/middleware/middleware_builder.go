package middleware

import (
	"fmt"
	"os"
	"path"
	"sort"

	"github.com/yusing/go-proxy/internal/gperr"
	"gopkg.in/yaml.v3"
)

var ErrMissingMiddlewareUse = gperr.New("missing middleware 'use' field")

func BuildMiddlewaresFromComposeFile(filePath string, eb *gperr.Builder) map[string]*Middleware {
	fileContent, err := os.ReadFile(filePath)
	if err != nil {
		eb.Add(err)
		return nil
	}
	return BuildMiddlewaresFromYAML(path.Base(filePath), fileContent, eb)
}

func BuildMiddlewaresFromYAML(source string, data []byte, eb *gperr.Builder) map[string]*Middleware {
	var rawMap map[string][]map[string]any
	err := yaml.Unmarshal(data, &rawMap)
	if err != nil {
		eb.Add(err)
		return nil
	}
	middlewares := make(map[string]*Middleware)
	for name, defs := range rawMap {
		chain, err := BuildMiddlewareFromChainRaw(name, defs)
		if err != nil {
			eb.Add(err.Subject(source))
		} else {
			middlewares[name+"@file"] = chain
		}
	}
	return middlewares
}

func compileMiddlewares(middlewaresMap map[string]OptionsRaw) ([]*Middleware, gperr.Error) {
	middlewares := make([]*Middleware, 0, len(middlewaresMap))

	errs := gperr.NewBuilder("middlewares compile error")
	invalidOpts := gperr.NewBuilder("options compile error")

	for name, opts := range middlewaresMap {
		m, err := Get(name)
		if err != nil {
			errs.Add(err)
			continue
		}

		m, err = m.New(opts)
		if err != nil {
			invalidOpts.Add(err.Subject(name))
			continue
		}
		middlewares = append(middlewares, m)
	}

	if invalidOpts.HasError() {
		errs.Add(invalidOpts.Error())
	}
	sort.Sort(ByPriority(middlewares))
	return middlewares, errs.Error()
}

func BuildMiddlewareFromMap(name string, middlewaresMap map[string]OptionsRaw) (*Middleware, gperr.Error) {
	compiled, err := compileMiddlewares(middlewaresMap)
	if err != nil {
		return nil, err
	}
	return NewMiddlewareChain(name, compiled), nil
}

// TODO: check conflict or duplicates.
func BuildMiddlewareFromChainRaw(name string, defs []map[string]any) (*Middleware, gperr.Error) {
	chainErr := gperr.NewBuilder("")
	chain := make([]*Middleware, 0, len(defs))
	for i, def := range defs {
		if def["use"] == nil || def["use"] == "" {
			chainErr.Add(ErrMissingMiddlewareUse.Subjectf("%s[%d]", name, i))
			continue
		}
		baseName := def["use"].(string)
		base, err := Get(baseName)
		if err != nil {
			chainErr.Add(err.Subjectf("%s[%d]", name, i))
			continue
		}
		delete(def, "use")
		m, err := base.New(def)
		if err != nil {
			chainErr.Add(err.Subjectf("%s[%d]", name, i))
			continue
		}
		m.name = fmt.Sprintf("%s[%d]", name, i)
		chain = append(chain, m)
	}
	if chainErr.HasError() {
		return nil, chainErr.Error()
	}
	return NewMiddlewareChain(name, chain), nil
}
