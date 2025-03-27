package rules

import (
	"net"
	"net/http"
	"net/url"
	"sync"
)

// Cache is a map of cached values for a request.
// It prevents the same value from being parsed multiple times.
type (
	Cache             map[string]any
	UpdateFunc[T any] func(T) T
)

const (
	CacheKeyQueries   = "queries"
	CacheKeyCookies   = "cookies"
	CacheKeyRemoteIP  = "remote_ip"
	CacheKeyBasicAuth = "basic_auth"
)

var cacheKeys = []string{
	CacheKeyQueries,
	CacheKeyCookies,
	CacheKeyRemoteIP,
	CacheKeyBasicAuth,
}

var cachePool = &sync.Pool{
	New: func() any {
		return make(Cache)
	},
}

// NewCache returns a new Cached.
func NewCache() Cache {
	return cachePool.Get().(Cache)
}

// Release clear the contents of the Cached and returns it to the pool.
func (c Cache) Release() {
	clear(c)
	cachePool.Put(c)
}

// GetQueries returns the queries.
// If r does not have queries, an empty map is returned.
func (c Cache) GetQueries(r *http.Request) url.Values {
	v, ok := c[CacheKeyQueries]
	if !ok {
		v = r.URL.Query()
		c[CacheKeyQueries] = v
	}
	return v.(url.Values)
}

func (c Cache) UpdateQueries(r *http.Request, update func(url.Values)) {
	queries := c.GetQueries(r)
	update(queries)
	r.URL.RawQuery = queries.Encode()
}

// GetCookies returns the cookies.
// If r does not have cookies, an empty slice is returned.
func (c Cache) GetCookies(r *http.Request) []*http.Cookie {
	v, ok := c[CacheKeyCookies]
	if !ok {
		v = r.Cookies()
		c[CacheKeyCookies] = v
	}
	return v.([]*http.Cookie)
}

func (c Cache) UpdateCookies(r *http.Request, update UpdateFunc[[]*http.Cookie]) {
	cookies := update(c.GetCookies(r))
	c[CacheKeyCookies] = cookies
	r.Header.Del("Cookie")
	for _, cookie := range cookies {
		r.AddCookie(cookie)
	}
}

// GetRemoteIP returns the remote ip address.
// If r.RemoteAddr is not a valid ip address, nil is returned.
func (c Cache) GetRemoteIP(r *http.Request) net.IP {
	v, ok := c[CacheKeyRemoteIP]
	if !ok {
		host, _, err := net.SplitHostPort(r.RemoteAddr)
		if err != nil {
			host = r.RemoteAddr
		}
		v = net.ParseIP(host)
		c[CacheKeyRemoteIP] = v
	}
	return v.(net.IP)
}

// GetBasicAuth returns *Credentials the basic auth username and password.
// If r does not have basic auth, nil is returned.
func (c Cache) GetBasicAuth(r *http.Request) *Credentials {
	v, ok := c[CacheKeyBasicAuth]
	if !ok {
		u, p, ok := r.BasicAuth()
		if ok {
			v = &Credentials{u, []byte(p)}
			c[CacheKeyBasicAuth] = v
		} else {
			c[CacheKeyBasicAuth] = nil
			return nil
		}
	}
	return v.(*Credentials)
}
