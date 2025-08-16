package api

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/rs/zerolog/log"
	swaggerFiles "github.com/swaggo/files"
	ginSwagger "github.com/swaggo/gin-swagger"
	apitypes "github.com/yusing/go-proxy/internal/api/types"
	apiV1 "github.com/yusing/go-proxy/internal/api/v1"
	agentApi "github.com/yusing/go-proxy/internal/api/v1/agent"
	authApi "github.com/yusing/go-proxy/internal/api/v1/auth"
	certApi "github.com/yusing/go-proxy/internal/api/v1/cert"
	dockerApi "github.com/yusing/go-proxy/internal/api/v1/docker"
	"github.com/yusing/go-proxy/internal/api/v1/docs"
	fileApi "github.com/yusing/go-proxy/internal/api/v1/file"
	homepageApi "github.com/yusing/go-proxy/internal/api/v1/homepage"
	metricsApi "github.com/yusing/go-proxy/internal/api/v1/metrics"
	routeApi "github.com/yusing/go-proxy/internal/api/v1/route"
	"github.com/yusing/go-proxy/internal/auth"
)

func NewHandler() *gin.Engine {
	gin.SetMode("release")
	r := gin.New()
	r.Use(ErrorHandler())
	r.Use(ErrorLoggingMiddleware())

	docs.SwaggerInfo.Title = "GoDoxy API"
	docs.SwaggerInfo.BasePath = "/api/v1"

	v1Auth := r.Group("/api/v1/auth")
	{
		v1Auth.HEAD("/check", authApi.Check)
		v1Auth.POST("/login", authApi.Login)
		v1Auth.POST("/callback", authApi.Callback)
		v1Auth.POST("/logout", authApi.Logout)
	}

	v1Swagger := r.Group("/api/v1/swagger")
	{
		v1Swagger.GET("/:any", func(c *gin.Context) {
			c.Redirect(http.StatusTemporaryRedirect, "/api/v1/swagger/index.html")
		})
		v1Swagger.GET("/:any/*any", ginSwagger.WrapHandler(swaggerFiles.Handler))
	}

	v1 := r.Group("/api/v1")
	v1.Use(AuthMiddleware())
	{
		v1.GET("/favicon", apiV1.FavIcon)
		v1.GET("/health", apiV1.Health)
		v1.GET("/icons", apiV1.Icons)
		v1.POST("/reload", apiV1.Reload)
		v1.GET("/stats", apiV1.Stats)
		v1.GET("/version", apiV1.Version)

		route := v1.Group("/route")
		{
			route.GET("/list", routeApi.Routes)
			route.GET("/:which", routeApi.Route)
			route.GET("/providers", routeApi.Providers)
			route.GET("/by_provider", routeApi.ByProvider)
		}

		file := v1.Group("/file")
		{
			file.GET("/list", fileApi.List)
			file.GET("/content", fileApi.Get)
			file.PUT("/content", fileApi.Set)
			file.POST("/content", fileApi.Set)
			file.POST("/validate", fileApi.Validate)
		}

		homepage := v1.Group("/homepage")
		{
			homepage.GET("/categories", homepageApi.Categories)
			homepage.GET("/items", homepageApi.Items)
			homepage.POST("/set/item", homepageApi.SetItem)
			homepage.POST("/set/items_batch", homepageApi.SetItemsBatch)
			homepage.POST("/set/item_visible", homepageApi.SetItemVisible)
			homepage.POST("/set/category_order", homepageApi.SetCategoryOrder)
		}

		cert := v1.Group("/cert")
		{
			cert.GET("/info", certApi.Info)
			cert.POST("/renew", certApi.Renew)
		}

		agent := v1.Group("/agent")
		{
			agent.GET("/list", agentApi.List)
			agent.POST("/create", agentApi.Create)
			agent.POST("/verify", agentApi.Verify)
		}

		metrics := v1.Group("/metrics")
		{
			metrics.GET("/system_info", metricsApi.SystemInfo)
			metrics.GET("/uptime", metricsApi.Uptime)
		}

		docker := v1.Group("/docker")
		{
			docker.GET("/containers", dockerApi.Containers)
			docker.GET("/info", dockerApi.Info)
			docker.GET("/logs/:server/:container", dockerApi.Logs)
		}
	}

	return r
}

func AuthMiddleware() gin.HandlerFunc {
	if !auth.IsEnabled() {
		return func(c *gin.Context) {
			c.Next()
		}
	}
	return func(c *gin.Context) {
		err := auth.GetDefaultAuth().CheckToken(c.Request)
		if err != nil {
			c.JSON(http.StatusUnauthorized, apitypes.Error("Unauthorized", err))
			c.Abort()
			return
		}
		c.Next()
	}
}

func ErrorHandler() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Next()
		if len(c.Errors) > 0 {
			for _, err := range c.Errors {
				log.Err(err.Err).Str("uri", c.Request.RequestURI).Msg("Internal error")
			}
			if !isWebSocketRequest(c) {
				c.JSON(http.StatusInternalServerError, apitypes.Error("Internal server error"))
			}
		}
	}
}

func ErrorLoggingMiddleware() gin.HandlerFunc {
	return gin.CustomRecoveryWithWriter(nil, func(c *gin.Context, err any) {
		log.Error().Any("error", err).Str("uri", c.Request.RequestURI).Msg("Internal error")
		if !isWebSocketRequest(c) {
			c.JSON(http.StatusInternalServerError, apitypes.Error("Internal server error"))
		}
	})
}

func isWebSocketRequest(c *gin.Context) bool {
	return c.GetHeader("Upgrade") == "websocket"
}
