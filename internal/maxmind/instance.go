package maxmind

import (
	"sync"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"github.com/yusing/godoxy/internal/notif"
	"github.com/yusing/goutils/task"
)

var instance *MaxMind

var warnOnce sync.Once

func warnNotConfigured() {
	log.Warn().Msg("MaxMind not configured, geo lookup will fail")
	notif.Notify(&notif.LogMessage{
		Level: zerolog.WarnLevel,
		Title: "MaxMind not configured",
		Body:  notif.MessageBody("MaxMind is not configured, geo lookup will fail"),
		Color: notif.ColorError,
	})
}

func SetInstance(parent task.Parent, cfg *Config) error {
	newInstance := &MaxMind{Config: cfg}
	if err := newInstance.LoadMaxMindDB(parent); err != nil {
		return err
	}
	instance = newInstance
	return nil
}

func HasInstance() bool {
	return instance != nil
}

func LookupCity(ip *IPInfo) (*City, bool) {
	if instance == nil {
		warnOnce.Do(warnNotConfigured)
		return nil, false
	}
	return instance.lookupCity(ip)
}
