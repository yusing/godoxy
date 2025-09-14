package auth

import (
	"github.com/gin-gonic/gin"
	"github.com/yusing/go-proxy/internal/auth"
)

// @x-id	  	"check"
// @Base			/api/v1
// @Summary		Check authentication status
// @Description	Checks if the user is authenticated by validating their token
// @Tags			auth
// @Produce		plain
// @Success		200	{string}	string	"OK"
// @Failure		302	{string}	string	"Redirects to login page or IdP"
// @Router			/auth/check [head]
func Check(c *gin.Context) {
	auth.AuthCheckHandler(c.Writer, c.Request)
}
