package middleware

import (
	"net/http"
	"strconv"
	"strings"
	"testing"

	. "github.com/yusing/go-proxy/internal/utils/testing"
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
		ExpectNoError(t, err)
		chain[i] = mid
	}
	res, err := newMiddlewaresTest(chain, nil)
	ExpectNoError(t, err)
	ExpectEqual(t, strings.Join(res.ResponseHeaders["Test-Value"], ","), "3,0,1,2")
}
