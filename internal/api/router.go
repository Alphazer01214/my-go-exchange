package api

import (
	"net/url"
	"strings"

	"github.com/gin-gonic/gin"
)

func (s *Server) registerRoutes(e *gin.Engine) {
	e.GET("/health", s.health)
	e.GET("/ws/v1", s.websocket)

	api := e.Group("/api")
	{
		api.GET("/instruments", s.listInstruments)
		api.GET("/book", s.getBook)
	}

	private := e.Group("/api")
	private.Use(s.requireAuth())
	{
		private.POST("/account/deposit", s.deposit)
		private.GET("/account/balances", s.listBalances)
		private.GET("/account/positions", s.listPositions)

		private.POST("/orders", s.placeOrder)
		private.GET("/orders/:orderID", s.getOrder)
		private.DELETE("/orders/:orderID", s.cancelOrder)
	}
}

func cors(allowed []string) gin.HandlerFunc {
	allowedOrigins := make(map[string]struct{}, len(allowed))
	allowAny := false
	for _, origin := range allowed {
		origin = strings.TrimSpace(origin)
		if origin == "*" {
			allowAny = true
		}
		if origin != "" {
			allowedOrigins[origin] = struct{}{}
		}
	}
	if len(allowedOrigins) == 0 {
		allowAny = true
	}

	return func(c *gin.Context) {
		origin := strings.TrimSpace(c.GetHeader("Origin"))
		switch {
		case allowAny:
			c.Header("Access-Control-Allow-Origin", "*")
		case originAllowed(origin, allowedOrigins, c.Request.Host):
			c.Header("Access-Control-Allow-Origin", origin)
			c.Header("Vary", "Origin")
		}
		c.Header("Access-Control-Allow-Headers", "Content-Type, Authorization")
		c.Header("Access-Control-Allow-Methods", "GET,POST,DELETE,OPTIONS")
		if c.Request.Method == "OPTIONS" {
			if origin != "" && c.Writer.Header().Get("Access-Control-Allow-Origin") == "" {
				c.AbortWithStatus(403)
				return
			}
			c.AbortWithStatus(204)
			return
		}
		c.Next()
	}
}

func originAllowed(origin string, allowed map[string]struct{}, requestHost string) bool {
	if origin == "" {
		return true
	}
	if _, ok := allowed[origin]; ok {
		return true
	}
	parsed, err := url.Parse(origin)
	return err == nil && strings.EqualFold(parsed.Host, requestHost)
}
