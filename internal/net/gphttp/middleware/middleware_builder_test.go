package middleware

import (
	_ "embed"
	"encoding/json"
	"testing"

	"github.com/yusing/go-proxy/internal/gperr"
	. "github.com/yusing/go-proxy/internal/utils/testing"
)

//go:embed test_data/middleware_compose.yml
var testMiddlewareCompose []byte

func TestBuild(t *testing.T) {
	errs := gperr.NewBuilder("")
	middlewares := BuildMiddlewaresFromYAML("", testMiddlewareCompose, errs)
	ExpectNoError(t, errs.Error())
	Must(json.MarshalIndent(middlewares, "", "  "))
	// t.Log(string(data))
	// TODO: test
}
