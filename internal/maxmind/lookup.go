package maxmind

import (
	"errors"
	"net"
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
	ip := net.ParseIP(ipStr)
	if ip == nil {
		return nil, ErrInvalidIP
	}
	err := cfg.db.Lookup(ip, city)
	if err != nil {
		return nil, err
	}
	return city, nil
}
