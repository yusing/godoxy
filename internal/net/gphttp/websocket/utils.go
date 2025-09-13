package websocket

import (
	"time"

	"github.com/gin-gonic/gin"
	apitypes "github.com/yusing/go-proxy/internal/api/types"
)

type DeduplicateFunc func(last, current any) bool

func PeriodicWrite(c *gin.Context, interval time.Duration, get func() (any, error), deduplicate ...DeduplicateFunc) {
	manager, err := NewManagerWithUpgrade(c)
	if err != nil {
		c.Error(apitypes.InternalServerError(err, "failed to upgrade to websocket"))
		return
	}
	defer manager.Close()
	err = manager.PeriodicWrite(interval, get, deduplicate...)
	if err != nil {
		c.Error(apitypes.InternalServerError(err, "failed to write to websocket"))
	}
}
