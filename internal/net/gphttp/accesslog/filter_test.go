package accesslog_test

import (
	"net/http"
	"testing"

	. "github.com/yusing/go-proxy/internal/net/gphttp/accesslog"
	"github.com/yusing/go-proxy/internal/utils/strutils"
	. "github.com/yusing/go-proxy/internal/utils/testing"
)

func TestStatusCodeFilter(t *testing.T) {
	values := []*StatusCodeRange{
		strutils.MustParse[*StatusCodeRange]("200-308"),
	}
	t.Run("positive", func(t *testing.T) {
		filter := &LogFilter[*StatusCodeRange]{}
		ExpectTrue(t, filter.CheckKeep(nil, nil))

		// keep any 2xx 3xx (inclusive)
		filter.Values = values
		ExpectFalse(t, filter.CheckKeep(nil, &http.Response{
			StatusCode: http.StatusForbidden,
		}))
		ExpectTrue(t, filter.CheckKeep(nil, &http.Response{
			StatusCode: http.StatusOK,
		}))
		ExpectTrue(t, filter.CheckKeep(nil, &http.Response{
			StatusCode: http.StatusMultipleChoices,
		}))
		ExpectTrue(t, filter.CheckKeep(nil, &http.Response{
			StatusCode: http.StatusPermanentRedirect,
		}))
	})

	t.Run("negative", func(t *testing.T) {
		filter := &LogFilter[*StatusCodeRange]{
			Negative: true,
		}
		ExpectFalse(t, filter.CheckKeep(nil, nil))

		// drop any 2xx 3xx (inclusive)
		filter.Values = values
		ExpectTrue(t, filter.CheckKeep(nil, &http.Response{
			StatusCode: http.StatusForbidden,
		}))
		ExpectFalse(t, filter.CheckKeep(nil, &http.Response{
			StatusCode: http.StatusOK,
		}))
		ExpectFalse(t, filter.CheckKeep(nil, &http.Response{
			StatusCode: http.StatusMultipleChoices,
		}))
		ExpectFalse(t, filter.CheckKeep(nil, &http.Response{
			StatusCode: http.StatusPermanentRedirect,
		}))
	})
}

func TestMethodFilter(t *testing.T) {
	t.Run("positive", func(t *testing.T) {
		filter := &LogFilter[HTTPMethod]{}
		ExpectTrue(t, filter.CheckKeep(&http.Request{
			Method: http.MethodGet,
		}, nil))
		ExpectTrue(t, filter.CheckKeep(&http.Request{
			Method: http.MethodPost,
		}, nil))

		// keep get only
		filter.Values = []HTTPMethod{http.MethodGet}
		ExpectTrue(t, filter.CheckKeep(&http.Request{
			Method: http.MethodGet,
		}, nil))
		ExpectFalse(t, filter.CheckKeep(&http.Request{
			Method: http.MethodPost,
		}, nil))
	})

	t.Run("negative", func(t *testing.T) {
		filter := &LogFilter[HTTPMethod]{
			Negative: true,
		}
		ExpectFalse(t, filter.CheckKeep(&http.Request{
			Method: http.MethodGet,
		}, nil))
		ExpectFalse(t, filter.CheckKeep(&http.Request{
			Method: http.MethodPost,
		}, nil))

		// drop post only
		filter.Values = []HTTPMethod{http.MethodPost}
		ExpectFalse(t, filter.CheckKeep(&http.Request{
			Method: http.MethodPost,
		}, nil))
		ExpectTrue(t, filter.CheckKeep(&http.Request{
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
	ExpectEqual(t, headerFoo[0].Key, "Foo")
	ExpectEqual(t, headerFoo[0].Value, "")
	headerFooBar := []*HTTPHeader{
		strutils.MustParse[*HTTPHeader]("Foo=bar"),
	}
	ExpectEqual(t, headerFooBar[0].Key, "Foo")
	ExpectEqual(t, headerFooBar[0].Value, "bar")

	t.Run("positive", func(t *testing.T) {
		filter := &LogFilter[*HTTPHeader]{}
		ExpectTrue(t, filter.CheckKeep(fooBar, nil))
		ExpectTrue(t, filter.CheckKeep(fooBaz, nil))

		// keep any foo
		filter.Values = headerFoo
		ExpectTrue(t, filter.CheckKeep(fooBar, nil))
		ExpectTrue(t, filter.CheckKeep(fooBaz, nil))

		// keep foo == bar
		filter.Values = headerFooBar
		ExpectTrue(t, filter.CheckKeep(fooBar, nil))
		ExpectFalse(t, filter.CheckKeep(fooBaz, nil))
	})
	t.Run("negative", func(t *testing.T) {
		filter := &LogFilter[*HTTPHeader]{
			Negative: true,
		}
		ExpectFalse(t, filter.CheckKeep(fooBar, nil))
		ExpectFalse(t, filter.CheckKeep(fooBaz, nil))

		// drop any foo
		filter.Values = headerFoo
		ExpectFalse(t, filter.CheckKeep(fooBar, nil))
		ExpectFalse(t, filter.CheckKeep(fooBaz, nil))

		// drop foo == bar
		filter.Values = headerFooBar
		ExpectFalse(t, filter.CheckKeep(fooBar, nil))
		ExpectTrue(t, filter.CheckKeep(fooBaz, nil))
	})
}

func TestCIDRFilter(t *testing.T) {
	cidr := []*CIDR{
		strutils.MustParse[*CIDR]("192.168.10.0/24"),
	}
	ExpectEqual(t, cidr[0].String(), "192.168.10.0/24")
	inCIDR := &http.Request{
		RemoteAddr: "192.168.10.1",
	}
	notInCIDR := &http.Request{
		RemoteAddr: "192.168.11.1",
	}

	t.Run("positive", func(t *testing.T) {
		filter := &LogFilter[*CIDR]{}
		ExpectTrue(t, filter.CheckKeep(inCIDR, nil))
		ExpectTrue(t, filter.CheckKeep(notInCIDR, nil))

		filter.Values = cidr
		ExpectTrue(t, filter.CheckKeep(inCIDR, nil))
		ExpectFalse(t, filter.CheckKeep(notInCIDR, nil))
	})

	t.Run("negative", func(t *testing.T) {
		filter := &LogFilter[*CIDR]{Negative: true}
		ExpectFalse(t, filter.CheckKeep(inCIDR, nil))
		ExpectFalse(t, filter.CheckKeep(notInCIDR, nil))

		filter.Values = cidr
		ExpectFalse(t, filter.CheckKeep(inCIDR, nil))
		ExpectTrue(t, filter.CheckKeep(notInCIDR, nil))
	})
}
