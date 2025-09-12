package serialization

import (
	"time"
	_ "unsafe"
)

//go:linkname unitMap time.unitMap
var unitMap map[string]uint64

const (
	unitDay   uint64 = 24 * uint64(time.Hour)
	unitWeek  uint64 = 7 * unitDay
	unitMonth uint64 = 30 * unitDay
)

func init() {
	unitMap["d"] = unitDay
	unitMap["w"] = unitWeek
	unitMap["M"] = unitMonth
}
