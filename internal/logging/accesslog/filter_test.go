package accesslog_test

import (
	"net"
	"net/http"
	"testing"

	. "github.com/yusing/go-proxy/internal/logging/accesslog"
	nettypes "github.com/yusing/go-proxy/internal/net/types"
	"github.com/yusing/go-proxy/internal/utils/strutils"
	expect "github.com/yusing/go-proxy/internal/utils/testing"
)

func TestStatusCodeFilter(t *testing.T) {
	values := []*StatusCodeRange{
		strutils.MustParse[*StatusCodeRange]("200-308"),
	}
	t.Run("positive", func(t *testing.T) {
		filter := &LogFilter[*StatusCodeRange]{}
		expect.True(t, filter.CheckKeep(nil, nil))

		// keep any 2xx 3xx (inclusive)
		filter.Values = values
		expect.False(t, filter.CheckKeep(nil, &http.Response{
			StatusCode: http.StatusForbidden,
		}))
		expect.True(t, filter.CheckKeep(nil, &http.Response{
			StatusCode: http.StatusOK,
		}))
		expect.True(t, filter.CheckKeep(nil, &http.Response{
			StatusCode: http.StatusMultipleChoices,
		}))
		expect.True(t, filter.CheckKeep(nil, &http.Response{
			StatusCode: http.StatusPermanentRedirect,
		}))
	})

	t.Run("negative", func(t *testing.T) {
		filter := &LogFilter[*StatusCodeRange]{
			Negative: true,
		}
		expect.False(t, filter.CheckKeep(nil, nil))

		// drop any 2xx 3xx (inclusive)
		filter.Values = values
		expect.True(t, filter.CheckKeep(nil, &http.Response{
			StatusCode: http.StatusForbidden,
		}))
		expect.False(t, filter.CheckKeep(nil, &http.Response{
			StatusCode: http.StatusOK,
		}))
		expect.False(t, filter.CheckKeep(nil, &http.Response{
			StatusCode: http.StatusMultipleChoices,
		}))
		expect.False(t, filter.CheckKeep(nil, &http.Response{
			StatusCode: http.StatusPermanentRedirect,
		}))
	})
}

func TestMethodFilter(t *testing.T) {
	t.Run("positive", func(t *testing.T) {
		filter := &LogFilter[HTTPMethod]{}
		expect.True(t, filter.CheckKeep(&http.Request{
			Method: http.MethodGet,
		}, nil))
		expect.True(t, filter.CheckKeep(&http.Request{
			Method: http.MethodPost,
		}, nil))

		// keep get only
		filter.Values = []HTTPMethod{http.MethodGet}
		expect.True(t, filter.CheckKeep(&http.Request{
			Method: http.MethodGet,
		}, nil))
		expect.False(t, filter.CheckKeep(&http.Request{
			Method: http.MethodPost,
		}, nil))
	})

	t.Run("negative", func(t *testing.T) {
		filter := &LogFilter[HTTPMethod]{
			Negative: true,
		}
		expect.False(t, filter.CheckKeep(&http.Request{
			Method: http.MethodGet,
		}, nil))
		expect.False(t, filter.CheckKeep(&http.Request{
			Method: http.MethodPost,
		}, nil))

		// drop post only
		filter.Values = []HTTPMethod{http.MethodPost}
		expect.False(t, filter.CheckKeep(&http.Request{
			Method: http.MethodPost,
		}, nil))
		expect.True(t, filter.CheckKeep(&http.Request{
			Method: http.MethodGet,
		}, nil))
	})
}

func TestHeaderFilter(t *testing.T) {
	fooBar := &http.Request{
		Header: http.Header{
			"Foo": []string{"bar"},
		},
	}
	fooBaz := &http.Request{
		Header: http.Header{
			"Foo": []string{"baz"},
		},
	}
	headerFoo := []*HTTPHeader{
		strutils.MustParse[*HTTPHeader]("Foo"),
	}
	expect.Equal(t, headerFoo[0].Key, "Foo")
	expect.Equal(t, headerFoo[0].Value, "")
	headerFooBar := []*HTTPHeader{
		strutils.MustParse[*HTTPHeader]("Foo=bar"),
	}
	expect.Equal(t, headerFooBar[0].Key, "Foo")
	expect.Equal(t, headerFooBar[0].Value, "bar")

	t.Run("positive", func(t *testing.T) {
		filter := &LogFilter[*HTTPHeader]{}
		expect.True(t, filter.CheckKeep(fooBar, nil))
		expect.True(t, filter.CheckKeep(fooBaz, nil))

		// keep any foo
		filter.Values = headerFoo
		expect.True(t, filter.CheckKeep(fooBar, nil))
		expect.True(t, filter.CheckKeep(fooBaz, nil))

		// keep foo == bar
		filter.Values = headerFooBar
		expect.True(t, filter.CheckKeep(fooBar, nil))
		expect.False(t, filter.CheckKeep(fooBaz, nil))
	})
	t.Run("negative", func(t *testing.T) {
		filter := &LogFilter[*HTTPHeader]{
			Negative: true,
		}
		expect.False(t, filter.CheckKeep(fooBar, nil))
		expect.False(t, filter.CheckKeep(fooBaz, nil))

		// drop any foo
		filter.Values = headerFoo
		expect.False(t, filter.CheckKeep(fooBar, nil))
		expect.False(t, filter.CheckKeep(fooBaz, nil))

		// drop foo == bar
		filter.Values = headerFooBar
		expect.False(t, filter.CheckKeep(fooBar, nil))
		expect.True(t, filter.CheckKeep(fooBaz, nil))
	})
}

func TestCIDRFilter(t *testing.T) {
	cidr := []*CIDR{{nettypes.CIDR{
		IP:   net.ParseIP("192.168.10.0"),
		Mask: net.CIDRMask(24, 32),
	}}}
	expect.Equal(t, cidr[0].String(), "192.168.10.0/24")
	inCIDR := &http.Request{
		RemoteAddr: "192.168.10.1",
	}
	notInCIDR := &http.Request{
		RemoteAddr: "192.168.11.1",
	}

	t.Run("positive", func(t *testing.T) {
		filter := &LogFilter[*CIDR]{}
		expect.True(t, filter.CheckKeep(inCIDR, nil))
		expect.True(t, filter.CheckKeep(notInCIDR, nil))

		filter.Values = cidr
		expect.True(t, filter.CheckKeep(inCIDR, nil))
		expect.False(t, filter.CheckKeep(notInCIDR, nil))
	})

	t.Run("negative", func(t *testing.T) {
		filter := &LogFilter[*CIDR]{Negative: true}
		expect.False(t, filter.CheckKeep(inCIDR, nil))
		expect.False(t, filter.CheckKeep(notInCIDR, nil))

		filter.Values = cidr
		expect.False(t, filter.CheckKeep(inCIDR, nil))
		expect.True(t, filter.CheckKeep(notInCIDR, nil))
	})
}
