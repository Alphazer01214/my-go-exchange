package web

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/websocket"

	"my-go-exchange/internal/market"
	"my-go-exchange/internal/md/natspub"
)

// Meta is static app info for the UI.
type Meta struct {
	Source    string   `json:"source"`
	Publisher string   `json:"publisher"`
	NATS      string   `json:"nats"`
	Front     string   `json:"front"`
	Symbols   []string `json:"symbols"`
	Stream    string   `json:"stream"`
	Subject   string   `json:"subject"`
}

// Server serves REST + WebSocket + static market UI.
type Server struct {
	hub  *market.Hub
	meta Meta
	log  *slog.Logger
	upg  websocket.Upgrader
}

func NewServer(hub *market.Hub, meta Meta, log *slog.Logger) *Server {
	if log == nil {
		log = slog.Default()
	}
	return &Server{
		hub:  hub,
		meta: meta,
		log:  log,
		upg: websocket.Upgrader{
			CheckOrigin: func(r *http.Request) bool { return true },
		},
	}
}

// Handler returns the root mux.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.Handle("/", http.FileServer(http.FS(staticFS())))
	mux.HandleFunc("/api/health", s.handleHealth)
	mux.HandleFunc("/api/quotes", s.handleQuotes)
	mux.HandleFunc("/api/quotes/", s.handleQuoteOne)
	mux.HandleFunc("/api/kline/", s.handleKline)
	mux.HandleFunc("/api/meta", s.handleMeta)
	mux.HandleFunc("/ws", s.handleWS)
	return mux
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"status":    "ok",
		"source":    s.meta.Source,
		"publisher": s.meta.Publisher,
		"nats":      s.meta.NATS,
		"front":     s.meta.Front,
		"symbols":   s.meta.Symbols,
	})
}

func (s *Server) handleMeta(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"stream":  natspub.StreamName,
		"subject": natspub.SubjectPrefix + ".>",
		"front":   s.meta.Front,
		"symbols": s.meta.Symbols,
		"nats":    s.meta.NATS,
	})
}

func (s *Server) handleQuotes(w http.ResponseWriter, r *http.Request) {
	quotes, tape, klines, total := s.hub.Snapshot()
	writeJSON(w, http.StatusOK, map[string]any{
		"count":  len(quotes),
		"total":  total,
		"quotes": quotes,
		"tape":   tape,
		"klines": klines,
	})
}

func (s *Server) handleQuoteOne(w http.ResponseWriter, r *http.Request) {
	id := lastPathSegment(r.URL.Path)
	tick, ok := s.hub.Quote(id)
	if !ok {
		http.NotFound(w, r)
		return
	}
	writeJSON(w, http.StatusOK, tick)
}

func (s *Server) handleKline(w http.ResponseWriter, r *http.Request) {
	id := lastPathSegment(r.URL.Path)
	bars := s.hub.Klines(id)
	writeJSON(w, http.StatusOK, map[string]any{
		"instrument_id": id,
		"interval":      "1m",
		"bars":          bars,
	})
}

func (s *Server) handleWS(w http.ResponseWriter, r *http.Request) {
	conn, err := s.upg.Upgrade(w, r, nil)
	if err != nil {
		s.log.Warn("ws upgrade", "err", err)
		return
	}
	defer conn.Close()

	// initial snapshot
	quotes, tape, klines, total := s.hub.Snapshot()
	_ = s.writeJSON(conn, map[string]any{
		"type": "snapshot",
		"data": map[string]any{
			"quotes": quotes,
			"tape":   tape,
			"klines": klines,
			"total":  total,
			"meta":   s.meta,
		},
	})

	events, unsub := s.hub.Subscribe()
	defer unsub()

	// reader: detect close
	done := make(chan struct{})
	var once sync.Once
	closeDone := func() { once.Do(func() { close(done) }) }
	go func() {
		defer closeDone()
		for {
			if _, _, err := conn.ReadMessage(); err != nil {
				return
			}
		}
	}()

	ping := time.NewTicker(20 * time.Second)
	defer ping.Stop()

	for {
		select {
		case <-done:
			return
		case ev, ok := <-events:
			if !ok {
				return
			}
			if err := s.writeJSON(conn, ev); err != nil {
				return
			}
		case <-ping.C:
			if err := conn.WriteControl(websocket.PingMessage, []byte("ping"), time.Now().Add(5*time.Second)); err != nil {
				return
			}
		}
	}
}

func (s *Server) writeJSON(conn *websocket.Conn, v any) error {
	buf, err := json.Marshal(v)
	if err != nil {
		return err
	}
	return conn.WriteMessage(websocket.TextMessage, buf)
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(code)
	enc := json.NewEncoder(w)
	_ = enc.Encode(v)
}

func lastPathSegment(p string) string {
	for i := len(p) - 1; i >= 0; i-- {
		if p[i] == '/' {
			return p[i+1:]
		}
	}
	return p
}
