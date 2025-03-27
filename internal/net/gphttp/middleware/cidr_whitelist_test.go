package middleware

import (
	_ "embed"
	"net"
	"net/http"
	"strings"
	"testing"

	"github.com/yusing/go-proxy/internal/gperr"
	"github.com/yusing/go-proxy/internal/utils"
	. "github.com/yusing/go-proxy/internal/utils/testing"
)

//go:embed test_data/cidr_whitelist_test.yml
var testCIDRWhitelistCompose []byte
var deny, accept *Middleware

func TestCIDRWhitelistValidation(t *testing.T) {
	const testMessage = "test-message"
	t.Run("valid", func(t *testing.T) {
		_, err := CIDRWhiteList.New(OptionsRaw{
			"allow":   []string{"192.168.2.100/32"},
			"message": testMessage,
		})
		ExpectNoError(t, err)
		_, err = CIDRWhiteList.New(OptionsRaw{
			"allow":   []string{"192.168.2.100/32"},
			"message": testMessage,
			"status":  403,
		})
		ExpectNoError(t, err)
		_, err = CIDRWhiteList.New(OptionsRaw{
			"allow":       []string{"192.168.2.100/32"},
			"message":     testMessage,
			"status_code": 403,
		})
		ExpectNoError(t, err)
	})
	t.Run("missing allow", func(t *testing.T) {
		_, err := CIDRWhiteList.New(OptionsRaw{
			"message": testMessage,
		})
		ExpectError(t, utils.ErrValidationError, err)
	})
	t.Run("invalid cidr", func(t *testing.T) {
		_, err := CIDRWhiteList.New(OptionsRaw{
			"allow":   []string{"192.168.2.100/123"},
			"message": testMessage,
		})
		ExpectErrorT[*net.ParseError](t, err)
	})
	t.Run("invalid status code", func(t *testing.T) {
		_, err := CIDRWhiteList.New(OptionsRaw{
			"allow":       []string{"192.168.2.100/32"},
			"status_code": 600,
			"message":     testMessage,
		})
		ExpectError(t, utils.ErrValidationError, err)
	})
}

func TestCIDRWhitelist(t *testing.T) {
	errs := gperr.NewBuilder("")
	mids := BuildMiddlewaresFromYAML("", testCIDRWhitelistCompose, errs)
	ExpectNoError(t, errs.Error())
	deny = mids["deny@file"]
	accept = mids["accept@file"]
	if deny == nil || accept == nil {
		panic("bug occurred")
	}

	t.Run("deny", func(t *testing.T) {
		t.Parallel()
		for range 10 {
			result, err := newMiddlewareTest(deny, nil)
			ExpectNoError(t, err)
			ExpectEqual(t, result.ResponseStatus, cidrWhitelistDefaults.StatusCode)
			ExpectEqual(t, strings.TrimSpace(string(result.Data)), cidrWhitelistDefaults.Message)
		}
	})

	t.Run("accept", func(t *testing.T) {
		t.Parallel()
		for range 10 {
			result, err := newMiddlewareTest(accept, nil)
			ExpectNoError(t, err)
			ExpectEqual(t, result.ResponseStatus, http.StatusOK)
		}
	})
}
