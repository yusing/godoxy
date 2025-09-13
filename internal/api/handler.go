package api

import (
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"github.com/rs/zerolog/log"
	apitypes "github.com/yusing/go-proxy/internal/api/types"
	apiV1 "github.com/yusing/go-proxy/internal/api/v1"
	agentApi "github.com/yusing/go-proxy/internal/api/v1/agent"
	authApi "github.com/yusing/go-proxy/internal/api/v1/auth"
	certApi "github.com/yusing/go-proxy/internal/api/v1/cert"
	dockerApi "github.com/yusing/go-proxy/internal/api/v1/docker"
	fileApi "github.com/yusing/go-proxy/internal/api/v1/file"
	homepageApi "github.com/yusing/go-proxy/internal/api/v1/homepage"
	metricsApi "github.com/yusing/go-proxy/internal/api/v1/metrics"
	routeApi "github.com/yusing/go-proxy/internal/api/v1/route"
	"github.com/yusing/go-proxy/internal/auth"
	"github.com/yusing/go-proxy/internal/common"
	"github.com/yusing/go-proxy/internal/gperr"
)

// @title           GoDoxy API
// @version         1.0
// @description     GoDoxy API
// @termsOfService  https://github.com/yusing/godoxy/blob/main/LICENSE

// @contact.name   Yusing
// @contact.url    https://github.com/yusing/godoxy/issues

// @license.name  MIT
// @license.url   https://github.com/yusing/godoxy/blob/main/LICENSE

// @BasePath  /api/v1

// @externalDocs.description  GoDoxy Docs
// @externalDocs.url          https://docs.godoxy.dev
func NewHandler() *gin.Engine {
	if !common.IsDebug {
		gin.SetMode("release")
	}
	r := gin.New()
	r.Use(ErrorHandler())
	r.Use(ErrorLoggingMiddleware())

	r.GET("/api/v1/version", apiV1.Version)

	if auth.IsEnabled() {
		v1Auth := r.Group("/api/v1/auth")
		{
			v1Auth.HEAD("/check", authApi.Check)
			v1Auth.POST("/login", authApi.Login)
			v1Auth.GET("/callback", authApi.Callback)
			v1Auth.POST("/callback", authApi.Callback)
			v1Auth.POST("/logout", authApi.Logout)
		}
	}

	v1 := r.Group("/api/v1")
	if auth.IsEnabled() {
		v1.Use(AuthMiddleware())
	}
	if common.APISkipOriginCheck {
		v1.Use(SkipOriginCheckMiddleware())
	}
	{
		// enable cache for favicon
		v1.GET("/favicon", apiV1.FavIcon).Use(Cache(time.Hour * 24))
		v1.GET("/health", apiV1.Health)
		v1.GET("/icons", apiV1.Icons)
		v1.POST("/reload", apiV1.Reload)
		v1.GET("/stats", apiV1.Stats)

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
			homepage.POST("/set/item_favorite", homepageApi.SetItemFavorite)
			homepage.POST("/set/item_sort_order", homepageApi.SetItemSortOrder)
			homepage.POST("/set/item_all_sort_order", homepageApi.SetItemAllSortOrder)
			homepage.POST("/set/item_fav_sort_order", homepageApi.SetItemFavSortOrder)
			homepage.POST("/set/category_order", homepageApi.SetCategoryOrder)
			homepage.POST("/item_click", homepageApi.ItemClick)
		}

		cert := v1.Group("/cert")
		{
			cert.GET("/info", certApi.Info)
			cert.GET("/renew", certApi.Renew)
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
			metrics.GET("/all_system_info", metricsApi.AllSystemInfo)
			metrics.GET("/uptime", metricsApi.Uptime)
		}

		docker := v1.Group("/docker")
		{
			docker.GET("/container/:id", dockerApi.GetContainer)
			docker.GET("/containers", dockerApi.Containers)
			docker.GET("/info", dockerApi.Info)
			docker.GET("/logs/:id", dockerApi.Logs)
			docker.POST("/start", dockerApi.Start)
			docker.POST("/stop", dockerApi.Stop)
			docker.POST("/restart", dockerApi.Restart)
		}
	}

	// disable cache by default
	r.Use(NoCache())
	return r
}

func NoCache() gin.HandlerFunc {
	return func(c *gin.Context) {
		// skip cache if Cache-Control header is set or if caching is explicitly enabled
		if !c.GetBool("cache_enabled") && c.Writer.Header().Get("Cache-Control") == "" {
			c.Header("Cache-Control", "no-cache, no-store, must-revalidate")
			c.Header("Pragma", "no-cache")
			c.Header("Expires", "0")
		}
		c.Next()
	}
}

func Cache(duration time.Duration) gin.HandlerFunc {
	return func(c *gin.Context) {
		// Signal to NoCache middleware that caching is intended
		c.Set("cache_enabled", true)
		// skip cache if Cache-Control header is set
		if c.Writer.Header().Get("Cache-Control") == "" {
			c.Header("Cache-Control", "public, max-age="+strconv.FormatFloat(duration.Seconds(), 'f', 0, 64)+", immutable")
			c.Header("Pragma", "public")
			c.Header("Expires", time.Now().Add(duration).Format(time.RFC1123))
		}
		c.Next()
	}
}

func AuthMiddleware() gin.HandlerFunc {
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

func SkipOriginCheckMiddleware() gin.HandlerFunc {
	upgrader := &websocket.Upgrader{
		CheckOrigin: func(r *http.Request) bool {
			return true
		},
	}
	return func(c *gin.Context) {
		c.Set("upgrader", upgrader)
		c.Next()
	}
}

func ErrorHandler() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Next()
		if len(c.Errors) > 0 {
			logger := log.With().Str("uri", c.Request.RequestURI).Logger()
			for _, err := range c.Errors {
				gperr.LogError("Internal error", err.Err, &logger)
			}
			if !c.IsWebsocket() {
				c.JSON(http.StatusInternalServerError, apitypes.Error("Internal server error"))
			}
		}
	}
}

func ErrorLoggingMiddleware() gin.HandlerFunc {
	return gin.CustomRecoveryWithWriter(nil, func(c *gin.Context, err any) {
		log.Error().Any("error", err).Str("uri", c.Request.RequestURI).Msg("Internal error")
		if !c.IsWebsocket() {
			c.JSON(http.StatusInternalServerError, apitypes.Error("Internal server error"))
		}
	})
}
