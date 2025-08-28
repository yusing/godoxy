//nolint:dupword
package auth

import (
	"github.com/gin-gonic/gin"
	"github.com/yusing/go-proxy/internal/auth"
)

// @x-id				"callback"
// @Base			/api/v1
// @Summary		Auth Callback
// @Description	Handles the callback from the provider after successful authentication
// @Tags			auth
// @Produce		plain
// @Param		  body	body	auth.UserPassAuthCallbackRequest	true	"Userpass only"
// @Success		200	{string}	string	"Userpass: OK"
// @Success		302	{string}	string	"OIDC: Redirects to home page"
// @Failure		400	{string}	string	"OIDC: invalid request (missing state cookie or oauth state)"
// @Failure		400	{string}	string	"Userpass: invalid request / credentials"
// @Failure		500	{string}	string	"Internal server error"
// @Router			/auth/callback [post]
func Callback(c *gin.Context) {
	auth.GetDefaultAuth().PostAuthCallbackHandler(c.Writer, c.Request)
}
