package api

import (
	"my-go-exchange/internal/market"
	"my-go-exchange/internal/stream"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/gorilla/websocket"
)

type Config struct {
	RateLimitSec   int
	Production     bool
	AllowedOrigins []string
	Authenticator  Authenticator
}

type Server struct {
	market  *market.Market
	streams *stream.Hub
	engine  *gin.Engine
	cfg     Config
	up      websocket.Upgrader
}

// NewServer creates the HTTP adapter for the exchange.
func NewServer(exchange *market.Market, cfg Config) *Server {
	if exchange == nil {
		panic("api: market is required")
	}
	if cfg.Production {
		gin.SetMode(gin.ReleaseMode)
	}

	engine := gin.New()
	engine.Use(gin.Logger(), gin.Recovery(), cors(cfg.AllowedOrigins))

	server := &Server{
		market:  exchange,
		streams: exchange.Streams(),
		engine:  engine,
		cfg:     cfg,
		up: websocket.Upgrader{
			CheckOrigin: websocketOriginCheck(cfg.AllowedOrigins),
		},
	}
	server.registerRoutes(engine)
	return server
}

func (s *Server) Handler() http.Handler {
	return s.engine
}
