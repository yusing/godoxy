package maxmind

import (
	"errors"
	"net/netip"
)

var ErrInvalidIP = errors.New("invalid IP address")
var ErrDBNotLoaded = errors.New("maxmind database not loaded")

func (cfg *MaxMind) lookupCityReal(ipStr string) (*City, error) {
	cfg.db.RLock()
	defer cfg.db.RUnlock()

	if cfg.db.Reader == nil {
		return nil, ErrDBNotLoaded
	}

	city := new(City)
	ip, err := netip.ParseAddr(ipStr)
	if err != nil || ip.Zone() != "" {
		return nil, ErrInvalidIP
	}
	err = cfg.db.Lookup(ip).Decode(city)
	if err != nil {
		return nil, err
	}
	return city, nil
}
