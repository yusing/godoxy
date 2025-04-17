package middleware

import (
	_ "embed"
	"testing"

	"github.com/yusing/go-proxy/pkg/json"

	"github.com/yusing/go-proxy/internal/gperr"
	expect "github.com/yusing/go-proxy/internal/utils/testing"
)

//go:embed test_data/middleware_compose.yml
var testMiddlewareCompose []byte

func TestBuild(t *testing.T) {
	errs := gperr.NewBuilder("")
	middlewares := BuildMiddlewaresFromYAML("", testMiddlewareCompose, errs)
	expect.NoError(t, errs.Error())
	json.Marshal(middlewares)
	// t.Log(string(data))
	// TODO: test
}
