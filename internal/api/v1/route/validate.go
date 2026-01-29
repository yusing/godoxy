package routeApi

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/goccy/go-yaml"
	"github.com/yusing/godoxy/internal/route"
	"github.com/yusing/godoxy/internal/serialization"
	apitypes "github.com/yusing/goutils/apitypes"
	"github.com/yusing/goutils/http/httpheaders"
	"github.com/yusing/goutils/http/websocket"
)

type _ = route.Route

// @x-id			"validate"
// @BasePath	/api/v1
// @Summary		Validate route
// @Description	Validate route,
// @Tags			route,websocket
// @Accept		application/yaml
// @Produce		json
// @Param			route body route.Route true "Route"
// @Success		200		{object}	apitypes.SuccessResponse "Route validated"
// @Failure		400		{object}	apitypes.ErrorResponse "Bad request"
// @Failure		403		{object}	apitypes.ErrorResponse "Forbidden"
// @Failure		417		{object}	any "Validation failed"
// @Failure		500		{object}	apitypes.ErrorResponse "Internal server error"
// @Router		/route/validate [get]
// @Router		/route/validate [post]
func Validate(c *gin.Context) {
	if httpheaders.IsWebsocket(c.Request.Header) {
		ValidateWS(c)
		return
	}
	var request route.Route
	if err := c.ShouldBindWith(&request, serialization.GinYAMLBinding{}); err != nil {
		c.JSON(http.StatusExpectationFailed, err)
		return
	}
	c.JSON(http.StatusOK, apitypes.Success("route validated"))
}

func ValidateWS(c *gin.Context) {
	manager, err := websocket.NewManagerWithUpgrade(c)
	if err != nil {
		c.Error(apitypes.InternalServerError(err, "failed to upgrade to websocket"))
		return
	}
	defer manager.Close()

	const writeTimeout = 5 * time.Second

	for {
		select {
		case <-manager.Done():
			return
		case msg := <-manager.ReadCh():
			var request route.Route
			if err := serialization.UnmarshalValidate(msg, &request, yaml.Unmarshal); err != nil {
				manager.WriteJSON(gin.H{"error": err}, writeTimeout)
				continue
			}
			manager.WriteJSON(gin.H{"message": "route validated"}, writeTimeout)
		}
	}
}
