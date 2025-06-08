package middleware

import (
	"net"
	"net/http"
	"strings"
	"testing"

	"github.com/yusing/go-proxy/internal/net/gphttp/httpheaders"
	nettypes "github.com/yusing/go-proxy/internal/net/types"
	. "github.com/yusing/go-proxy/internal/utils/testing"
)

func TestSetRealIPOpts(t *testing.T) {
	opts := OptionsRaw{
		"header": httpheaders.HeaderXRealIP,
		"from": []string{
			"127.0.0.0/8",
			"192.168.0.0/16",
			"172.16.0.0/12",
		},
		"recursive": true,
	}
	optExpected := &RealIPOpts{
		Header: httpheaders.HeaderXRealIP,
		From: []*nettypes.CIDR{
			{
				IP:   net.ParseIP("127.0.0.0"),
				Mask: net.IPv4Mask(255, 0, 0, 0),
			},
			{
				IP:   net.ParseIP("192.168.0.0"),
				Mask: net.IPv4Mask(255, 255, 0, 0),
			},
			{
				IP:   net.ParseIP("172.16.0.0"),
				Mask: net.IPv4Mask(255, 240, 0, 0),
			},
		},
		Recursive: true,
	}

	ri, err := RealIP.New(opts)
	ExpectNoError(t, err)
	ExpectEqual(t, ri.impl.(*realIP).Header, optExpected.Header)
	ExpectEqual(t, ri.impl.(*realIP).Recursive, optExpected.Recursive)
	for i, CIDR := range ri.impl.(*realIP).From {
		ExpectEqual(t, CIDR.String(), optExpected.From[i].String())
	}
}

func TestSetRealIP(t *testing.T) {
	const (
		testHeader = httpheaders.HeaderXRealIP
		testRealIP = "192.168.1.1"
	)
	opts := OptionsRaw{
		"header": testHeader,
		"from":   []string{"0.0.0.0/0"},
	}
	optsMr := OptionsRaw{
		"set_headers": map[string]string{testHeader: testRealIP},
	}
	realip, err := RealIP.New(opts)
	ExpectNoError(t, err)

	mr, err := ModifyRequest.New(optsMr)
	ExpectNoError(t, err)

	mid := NewMiddlewareChain("test", []*Middleware{mr, realip})

	result, err := newMiddlewareTest(mid, nil)
	ExpectNoError(t, err)
	ExpectEqual(t, result.ResponseStatus, http.StatusOK)
	ExpectEqual(t, strings.Split(result.RemoteAddr, ":")[0], testRealIP)
}
