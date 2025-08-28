package websocket

import (
	"time"

	"github.com/gin-gonic/gin"
	apitypes "github.com/yusing/go-proxy/internal/api/types"
)

func PeriodicWrite(c *gin.Context, interval time.Duration, get func() (any, error)) {
	manager, err := NewManagerWithUpgrade(c)
	if err != nil {
		c.Error(apitypes.InternalServerError(err, "failed to upgrade to websocket"))
		return
	}
	defer manager.Close()
	err = manager.PeriodicWrite(interval, get)
	if err != nil {
		c.Error(apitypes.InternalServerError(err, "failed to write to websocket"))
	}
}
