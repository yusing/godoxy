package maxmind

import (
	"github.com/yusing/go-proxy/internal/gperr"
	"github.com/yusing/go-proxy/internal/task"
)

var instance *MaxMind

func SetInstance(parent task.Parent, cfg *Config) gperr.Error {
	newInstance := &MaxMind{Config: cfg}
	if err := newInstance.LoadMaxMindDB(parent); err != nil {
		return err
	}
	if instance != nil {
		instance.task.Finish("updated")
	}
	instance = newInstance
	return nil
}

func HasInstance() bool {
	return instance != nil
}

func LookupCity(ip *IPInfo) (*City, bool) {
	if instance == nil {
		return nil, false
	}
	return instance.lookupCity(ip)
}
