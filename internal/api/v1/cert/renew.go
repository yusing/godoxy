package certapi

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/rs/zerolog/log"
	apitypes "github.com/yusing/godoxy/internal/api/types"
	config "github.com/yusing/godoxy/internal/config/types"
	"github.com/yusing/godoxy/internal/gperr"
	"github.com/yusing/godoxy/internal/logging/memlogger"
	"github.com/yusing/godoxy/internal/net/gphttp/websocket"
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
	autocert := config.GetInstance().AutoCertProvider()
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

	done := make(chan struct{})

	go func() {
		defer close(done)

		err = autocert.ObtainCert()
		if err != nil {
			gperr.LogError("failed to obtain cert", err)
			_ = manager.WriteData(websocket.TextMessage, []byte(err.Error()), 10*time.Second)
		} else {
			log.Info().Msg("cert obtained successfully")
		}
	}()

	for {
		select {
		case l := <-logs:
			if err != nil {
				return
			}

			err = manager.WriteData(websocket.TextMessage, l, 10*time.Second)
			if err != nil {
				return
			}
		case <-done:
			return
		}
	}
}
