package middleware

import (
	_ "embed"
	"encoding/json"
	"testing"

	gperr "github.com/yusing/goutils/errs"
	expect "github.com/yusing/goutils/testing"
)

//go:embed test_data/middleware_compose.yml
var testMiddlewareCompose []byte

func TestBuild(t *testing.T) {
	errs := gperr.NewBuilder("")
	middlewares := BuildMiddlewaresFromYAML("", testMiddlewareCompose, errs)
	expect.NoError(t, errs.Error())
	expect.Must(json.MarshalIndent(middlewares, "", "  "))
	// t.Log(string(data))
	// TODO: test
}
