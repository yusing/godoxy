package middleware

import (
	"net"
	"net/http"
	"sync"
	"time"

	"golang.org/x/time/rate"
)

type (
	requestMap  = map[string]*rate.Limiter
	rateLimiter struct {
		RateLimiterOpts
		Tracer

		requestMap requestMap
		mu         sync.Mutex
	}

	RateLimiterOpts struct {
		Average int           `validate:"min=1,required"`
		Burst   int           `validate:"min=1,required"`
		Period  time.Duration `validate:"min=1s"`
	}
)

var (
	RateLimiter            = NewMiddleware[rateLimiter]()
	rateLimiterOptsDefault = RateLimiterOpts{
		Period: time.Second,
	}
)

// setup implements MiddlewareWithSetup.
func (rl *rateLimiter) setup() {
	rl.RateLimiterOpts = rateLimiterOptsDefault
	rl.requestMap = make(requestMap, 0)
}

// before implements RequestModifier.
func (rl *rateLimiter) before(w http.ResponseWriter, r *http.Request) bool {
	return rl.limit(w, r)
}

func (rl *rateLimiter) newLimiter() *rate.Limiter {
	return rate.NewLimiter(rate.Limit(rl.Average)*rate.Every(rl.Period), rl.Burst)
}

func (rl *rateLimiter) limit(w http.ResponseWriter, r *http.Request) bool {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		rl.AddTracef("unable to parse remote address %s", r.RemoteAddr)
		http.Error(w, "Internal error", http.StatusInternalServerError)
		return false
	}

	rl.mu.Lock()
	limiter, ok := rl.requestMap[host]
	if !ok {
		limiter = rl.newLimiter()
		rl.requestMap[host] = limiter
	}
	rl.mu.Unlock()

	if limiter.Allow() {
		return true
	}

	http.Error(w, "rate limit exceeded", http.StatusTooManyRequests)
	return false
}
