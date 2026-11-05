package middleware

import (
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	nettypes "github.com/yusing/godoxy/internal/net/types"
	"github.com/yusing/godoxy/internal/route/rules"
	"github.com/yusing/goutils/http/httpheaders"
	expect "github.com/yusing/goutils/testing"
)

func TestRealIPBypassRefreshesCachedClientIP(t *testing.T) {
	mid, err := BuildMiddlewareFromChainRaw("test", []map[string]any{
		{"use": "real_ip", "from": []string{"127.0.0.1"}, "bypass": []string{"remote 192.0.2.123"}},
		{"use": "cidr_whitelist", "allow": []string{"10.0.0.0/8"}, "bypass": []string{"remote 127.0.0.1"}},
	})
	expect.NoError(t, err)
	var routeRules rules.Rules
	expect.NoError(t, routeRules.Parse("path / {\n upstream\n}"))
	reached := false
	handler := routeRules.BuildHandler(func(w http.ResponseWriter, r *http.Request) {
		mid.ModifyRequest(func(w http.ResponseWriter, _ *http.Request) {
			reached = true
			w.WriteHeader(http.StatusNoContent)
		}, w, r)
	})
	req := httptest.NewRequest(http.MethodGet, "http://example.com/", nil)
	req.RemoteAddr = "127.0.0.1:1234"
	req.Header.Set(httpheaders.HeaderXRealIP, "203.0.113.42")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	expect.Equal(t, w.Code, http.StatusForbidden)
	expect.Equal(t, reached, false)
	expect.Equal(t, req.RemoteAddr, "203.0.113.42:1234")
}

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
	expect.NoError(t, err)
	expect.Equal(t, ri.impl.(*realIP).Header, optExpected.Header)
	expect.Equal(t, ri.impl.(*realIP).Recursive, optExpected.Recursive)
	for i, CIDR := range ri.impl.(*realIP).From {
		expect.Equal(t, CIDR.String(), optExpected.From[i].String())
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
	expect.NoError(t, err)

	mr, err := ModifyRequest.New(optsMr)
	expect.NoError(t, err)

	mid := NewMiddlewareChain("test", []*Middleware{mr, realip})

	result, err := newMiddlewareTest(mid, nil)
	expect.NoError(t, err)
	expect.Equal(t, result.ResponseStatus, http.StatusOK)
	expect.Equal(t, strings.Split(result.RemoteAddr, ":")[0], testRealIP)
}

func TestSetRealIPSkipsInvalidRemoteAddr(t *testing.T) {
	realip, err := RealIP.New(OptionsRaw{
		"header": httpheaders.HeaderXRealIP,
		"from":   []string{"127.0.0.0/8"},
	})
	expect.NoError(t, err)

	result, err := newMiddlewareTest(realip, &testArgs{
		remoteAddr: "not-an-ip",
		headers: http.Header{
			httpheaders.HeaderXRealIP: []string{"192.168.1.1"},
		},
	})
	expect.NoError(t, err)
	expect.Equal(t, result.ResponseStatus, http.StatusOK)
	expect.Equal(t, result.RemoteAddr, "not-an-ip")
}

func TestSetRealIPSkipsInvalidHeaderValue(t *testing.T) {
	realip, err := RealIP.New(OptionsRaw{
		"header": httpheaders.HeaderXRealIP,
		"from":   []string{"127.0.0.0/8"},
	})
	expect.NoError(t, err)

	result, err := newMiddlewareTest(realip, &testArgs{
		remoteAddr: "127.0.0.1:1234",
		headers: http.Header{
			httpheaders.HeaderXRealIP: []string{"not-an-ip"},
		},
	})
	expect.NoError(t, err)
	expect.Equal(t, result.ResponseStatus, http.StatusOK)
	expect.Equal(t, result.RemoteAddr, "127.0.0.1:1234")
}
