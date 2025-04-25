package acl

import (
	"github.com/puzpuzpuz/xsync/v3"
	acl "github.com/yusing/go-proxy/internal/acl/types"
	"go.uber.org/atomic"
)

var cityCache = xsync.NewMapOf[string, *acl.City]()
var numCachedLookup atomic.Uint64

func (cfg *MaxMindConfig) lookupCity(ip *acl.IPInfo) (*acl.City, bool) {
	if ip.City != nil {
		return ip.City, true
	}

	if cfg.db.Reader == nil {
		return nil, false
	}

	city, ok := cityCache.Load(ip.Str)
	if ok {
		numCachedLookup.Inc()
		return city, true
	}

	cfg.db.RLock()
	defer cfg.db.RUnlock()

	city = new(acl.City)
	err := cfg.db.Lookup(ip.IP, city)
	if err != nil {
		return nil, false
	}

	cityCache.Store(ip.Str, city)
	ip.City = city
	return city, true
}
