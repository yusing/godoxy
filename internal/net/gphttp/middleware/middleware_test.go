package middleware

import (
	"net/http"
	"strconv"
	"strings"
	"testing"

	expect "github.com/yusing/goutils/testing"
)

type testPriority struct {
	Value int `json:"value"`
}

var test = NewMiddleware[testPriority]()

func (t testPriority) before(w http.ResponseWriter, r *http.Request) bool {
	w.Header().Add("Test-Value", strconv.Itoa(t.Value))
	return true
}

func TestMiddlewarePriority(t *testing.T) {
	priorities := []int{4, 7, 9, 0}
	chain := make([]*Middleware, len(priorities))
	for i, p := range priorities {
		mid, err := test.New(OptionsRaw{
			"priority": p,
			"value":    i,
		})
		expect.NoError(t, err)
		chain[i] = mid
	}
	res, err := newMiddlewaresTest(chain, nil)
	expect.NoError(t, err)
	expect.Equal(t, strings.Join(res.ResponseHeaders["Test-Value"], ","), "3,0,1,2")
}
