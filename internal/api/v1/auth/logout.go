package auth

import (
	"github.com/gin-gonic/gin"
	"github.com/yusing/go-proxy/internal/auth"
)

// @x-id				"logout"
// @Base			/api/v1
// @Summary		Logout
// @Description	Logs out the user by invalidating the token
// @Tags			auth
// @Produce		plain
// @Success		302	{string} string	"Redirects to home page"
// @Router			/auth/logout [post]
func Logout(c *gin.Context) {
	auth.GetDefaultAuth().LogoutHandler(c.Writer, c.Request)
}
