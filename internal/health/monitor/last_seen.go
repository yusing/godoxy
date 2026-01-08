package monitor

import (
	"time"

	"github.com/puzpuzpuz/xsync/v4"
)

var lastSeenMap = xsync.NewMap[string, time.Time](xsync.WithPresize(50), xsync.WithGrowOnly())

func SetLastSeen(service string, lastSeen time.Time) {
	lastSeenMap.Store(service, lastSeen)
}

func UpdateLastSeen(service string) {
	SetLastSeen(service, time.Now())
}

func GetLastSeen(service string) time.Time {
	lastSeen, _ := lastSeenMap.Load(service)
	return lastSeen
}
