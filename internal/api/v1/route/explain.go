package routeApi

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	entrypoint "github.com/yusing/godoxy/internal/entrypoint"
	apitypes "github.com/yusing/goutils/apitypes"
)

type ExplainRequest struct {
	// Peer address of the connection (e.g. 127.0.0.1 for a local tunnel).
	IP   string `form:"ip" binding:"required"`
	Host string `form:"host" binding:"required"`
	Path string `form:"path"`
	// GET by default.
	Method string `form:"method"`
	// https (default) or http.
	Scheme string `form:"scheme"`
	// Request headers as "Name: value", repeatable, e.g. "CF-Connecting-IP: 198.51.100.10".
	Headers []string `form:"header"`
} //	@name	ExplainRequest

// @x-id				"explain"
// @BasePath		/api/v1
// @Summary		Explain access decision
// @Description	Dry-run a request through the ACL, the entrypoint middlewares and the route middlewares, without reaching the upstream. Middlewares with side effects (auth, CrowdSec, rate limit) are reported as not evaluated.
// @Tags			route
// @Produce		json
// @Param			ip		query		string		true	"Peer IP of the connection"
// @Param			host	query		string		true	"Host header"
// @Param			path	query		string		false	"Request path"	default(/)
// @Param			method	query		string		false	"HTTP method"	default(GET)
// @Param			scheme	query		string		false	"http or https"	default(https)
// @Param			header	query		[]string	false	"Request header as Name: value, repeatable"	collectionFormat(multi)
// @Success		200		{object}	entrypoint.ExplainResult
// @Failure		400		{object}	apitypes.ErrorResponse
// @Failure		403		{object}	apitypes.ErrorResponse
// @Router			/route/explain [get]
func Explain(c *gin.Context) {
	var request ExplainRequest
	if err := c.ShouldBindQuery(&request); err != nil {
		c.JSON(http.StatusBadRequest, apitypes.Error("invalid request", err))
		return
	}

	headers := make(http.Header, len(request.Headers))
	for _, h := range request.Headers {
		name, value, ok := strings.Cut(h, ":")
		if !ok || strings.TrimSpace(name) == "" {
			c.JSON(http.StatusBadRequest, apitypes.Error("invalid header, expect \"Name: value\""))
			return
		}
		headers.Add(strings.TrimSpace(name), strings.TrimSpace(value))
	}

	ep, ok := entrypoint.FromCtx(c.Request.Context()).(interface {
		Explain(entrypoint.ExplainRequest) (*entrypoint.ExplainResult, error)
	})
	if !ok {
		c.JSON(http.StatusInternalServerError, apitypes.Error("entrypoint not initialized"))
		return
	}

	result, err := ep.Explain(entrypoint.ExplainRequest{
		IP:      request.IP,
		Host:    request.Host,
		Path:    request.Path,
		Method:  request.Method,
		Scheme:  request.Scheme,
		Headers: headers,
	})
	if err != nil {
		c.JSON(http.StatusBadRequest, apitypes.Error("invalid request", err))
		return
	}
	c.JSON(http.StatusOK, result)
}
