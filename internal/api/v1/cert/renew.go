package certapi

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/rs/zerolog/log"
	"github.com/yusing/godoxy/internal/autocert"
	"github.com/yusing/godoxy/internal/logging/memlogger"
	apitypes "github.com/yusing/goutils/apitypes"
	"github.com/yusing/goutils/http/websocket"
)

// @x-id				"renew"
// @BasePath		/api/v1
// @Summary		Renew cert
// @Description	Renew cert
// @Tags			cert,websocket
// @Produce		plain
// @Success		200	{object}	apitypes.SuccessResponse
// @Failure		403	{object}	apitypes.ErrorResponse
// @Failure		500	{object}	apitypes.ErrorResponse
// @Router			/cert/renew [get]
func Renew(c *gin.Context) {
	autocert := autocert.ActiveProvider.Load()
	if autocert == nil {
		c.JSON(http.StatusNotFound, apitypes.Error("autocert is not enabled"))
		return
	}

	manager, err := websocket.NewManagerWithUpgrade(c)
	if err != nil {
		c.Error(apitypes.InternalServerError(err, "failed to create websocket manager"))
		return
	}
	defer manager.Close()

	logs, cancel := memlogger.Events()
	defer cancel()

	go func() {
		// Stream logs until WebSocket connection closes (renewal runs in background)
		for {
			select {
			case <-manager.Context().Done():
				return
			case l := <-logs:
				if err != nil {
					return
				}

				err = manager.WriteData(websocket.TextMessage, l, 10*time.Second)
				if err != nil {
					return
				}
			}
		}
	}()

	// renewal happens in background
	ok := autocert.ForceExpiryAll()
	if !ok {
		log.Error().Msg("cert renewal already in progress")
		time.Sleep(1 * time.Second) // wait for the log above to be sent
		return
	}
	log.Info().Msg("cert force renewal requested")

	autocert.WaitRenewalDone(manager.Context())
}
