package auth

import (
	"github.com/gin-gonic/gin"
	"github.com/yusing/godoxy/internal/auth"
)

// @x-id       "login"
// @Base       /api/v1
// @Summary    Login
// @Description Initiates the login process by redirecting the user to the provider's login page
// @Tags       auth
// @Produce    plain
// @Success    302 {string} string "Redirects to login page or IdP"
// @Failure    429 {string} string "Too Many Requests"
// @Router     /auth/login [post]
func Login(c *gin.Context) {
	auth.GetDefaultAuth().LoginHandler(c.Writer, c.Request)
}
