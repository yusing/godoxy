package maxmind

import (
	"github.com/puzpuzpuz/xsync/v4"
)

var cityCache = xsync.NewMap[string, *City]()

func (cfg *MaxMind) lookupCity(ip *IPInfo) (*City, bool) {
	if ip.City != nil {
		return ip.City, true
	}

	if cfg.db.Reader == nil {
		return nil, false
	}

	city, ok := cityCache.Load(ip.Str)
	if ok {
		ip.City = city
		return city, true
	}

	cfg.db.RLock()
	defer cfg.db.RUnlock()

	city = new(City)
	err := cfg.db.Lookup(ip.IP, city)
	if err != nil {
		return nil, false
	}

	cityCache.Store(ip.Str, city)
	ip.City = city
	return city, true
}
