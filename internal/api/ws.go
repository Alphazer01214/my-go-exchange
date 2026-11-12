package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"my-go-exchange/internal/stream"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
)

const (
	wsReadLimit        = 4096
	wsWriteWait        = 10 * time.Second
	wsPongWait         = 60 * time.Second
	wsPingPeriod       = 25 * time.Second
	wsOutgoingBuffer   = 256
	wsSubscriptionsMax = 32
)

type wsClient struct {
	server *Server
	conn   *websocket.Conn

	outgoing chan []byte
	done     chan struct{}
	once     sync.Once

	mu            sync.Mutex
	accountID     int64
	subscriptions map[string]*stream.Subscription
}

func (s *Server) websocket(c *gin.Context) {
	conn, err := s.up.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		return
	}
	client := &wsClient{
		server:        s,
		conn:          conn,
		outgoing:      make(chan []byte, wsOutgoingBuffer),
		done:          make(chan struct{}),
		subscriptions: make(map[string]*stream.Subscription),
	}
	go client.writeLoop()
	client.readLoop()
	client.close()
}

func (c *wsClient) readLoop() {
	c.conn.SetReadLimit(wsReadLimit)
	_ = c.conn.SetReadDeadline(time.Now().Add(wsPongWait))
	c.conn.SetPongHandler(func(string) error {
		return c.conn.SetReadDeadline(time.Now().Add(wsPongWait))
	})

	for {
		messageType, payload, err := c.conn.ReadMessage()
		if err != nil {
			return
		}
		if messageType != websocket.TextMessage {
			c.sendError("", "unsupported_message", "only text messages are supported")
			continue
		}

		var request wsRequest
		if err := json.Unmarshal(payload, &request); err != nil {
			c.sendError("", "invalid_request", err.Error())
			continue
		}
		if err := c.handleRequest(request); err != nil {
			c.sendError(request.RequestID, "request_failed", err.Error())
		}
	}
}

func (c *wsClient) handleRequest(request wsRequest) error {
	switch request.Op {
	case wsOpPing:
		return c.sendJSON(wsResponse{
			Type:      "pong",
			RequestID: request.RequestID,
			Data:      map[string]int64{"server_time": time.Now().UnixMilli()},
		})
	case wsOpAuth:
		return c.authenticate(request)
	case wsOpSubscribe:
		return c.subscribe(request)
	case wsOpUnsubscribe:
		return c.unsubscribe(request)
	default:
		return errors.New("unsupported op")
	}
}

func (c *wsClient) authenticate(request wsRequest) error {
	if request.RequestID == "" {
		return errors.New("request_id is required")
	}
	if c.server.cfg.Authenticator == nil {
		return errors.New("authentication is unavailable")
	}
	accountID, err := c.server.cfg.Authenticator.Authenticate(context.Background(), request.Token)
	if err != nil {
		return err
	}
	if accountID <= 0 {
		return errors.New("invalid account")
	}

	c.mu.Lock()
	if c.accountID > 0 && c.accountID != accountID {
		c.mu.Unlock()
		return errors.New("connection is already authenticated as another account")
	}
	c.accountID = accountID
	c.mu.Unlock()
	return c.sendJSON(wsResponse{
		Type:      "ack",
		RequestID: request.RequestID,
		Data:      map[string]string{"op": wsOpAuth},
	})
}

func (c *wsClient) subscribe(request wsRequest) error {
	if request.RequestID == "" {
		return errors.New("request_id is required")
	}
	if len(request.Streams) == 0 || len(request.Streams) > wsSubscriptionsMax {
		return errors.New("invalid stream count")
	}

	c.mu.Lock()
	if _, exists := c.subscriptions[request.RequestID]; exists {
		c.mu.Unlock()
		return errors.New("request_id already subscribed")
	}
	accountID := c.accountID
	c.mu.Unlock()

	subscription, err := c.server.streams.SnapshotAndSubscribe(
		context.Background(),
		stream.SubscribeRequest{
			AccountID: accountID,
			Streams:   request.Streams,
		},
	)
	if err != nil {
		return err
	}

	c.mu.Lock()
	if _, exists := c.subscriptions[request.RequestID]; exists {
		c.mu.Unlock()
		subscription.Close()
		return errors.New("request_id already subscribed")
	}
	c.subscriptions[request.RequestID] = subscription
	c.mu.Unlock()

	go c.forwardSubscription(subscription)
	return c.sendJSON(wsResponse{
		Type:      "ack",
		RequestID: request.RequestID,
		Data: map[string]interface{}{
			"op":              wsOpSubscribe,
			"subscription_id": request.RequestID,
			"streams":         request.Streams,
		},
	})
}

func (c *wsClient) unsubscribe(request wsRequest) error {
	if request.RequestID == "" {
		return errors.New("request_id is required")
	}
	id := request.SubscriptionID
	if id == "" {
		id = request.RequestID
	}

	c.mu.Lock()
	subscription := c.subscriptions[id]
	delete(c.subscriptions, id)
	c.mu.Unlock()
	if subscription == nil {
		return errors.New("subscription not found")
	}
	subscription.Close()
	return c.sendJSON(wsResponse{
		Type:      "ack",
		RequestID: request.RequestID,
		Data:      map[string]string{"op": wsOpUnsubscribe},
	})
}

func (c *wsClient) forwardSubscription(subscription *stream.Subscription) {
	for payload := range subscription.Messages {
		if !c.send(payload) {
			return
		}
	}
	if err := subscription.Err(); err != nil && !errors.Is(err, stream.ErrSubscriptionClosed) {
		c.close()
	}
}

func (c *wsClient) writeLoop() {
	ticker := time.NewTicker(wsPingPeriod)
	defer ticker.Stop()
	defer c.close()

	for {
		select {
		case payload := <-c.outgoing:
			_ = c.conn.SetWriteDeadline(time.Now().Add(wsWriteWait))
			if err := c.conn.WriteMessage(websocket.TextMessage, payload); err != nil {
				return
			}
		case <-ticker.C:
			_ = c.conn.SetWriteDeadline(time.Now().Add(wsWriteWait))
			if err := c.conn.WriteControl(
				websocket.PingMessage,
				nil,
				time.Now().Add(wsWriteWait),
			); err != nil {
				return
			}
		case <-c.done:
			return
		}
	}
}

func (c *wsClient) send(payload []byte) bool {
	if len(payload) == 0 {
		return true
	}
	select {
	case <-c.done:
		return false
	case c.outgoing <- payload:
		return true
	default:
		c.close()
		return false
	}
}

func (c *wsClient) sendJSON(response wsResponse) error {
	payload, err := json.Marshal(response)
	if err != nil {
		return err
	}
	if !c.send(payload) {
		return errors.New("connection is closed")
	}
	return nil
}

func (c *wsClient) sendError(requestID, code, message string) {
	_ = c.sendJSON(wsResponse{
		Type:      "error",
		RequestID: requestID,
		Error:     &wsError{Code: code, Message: message},
	})
}

func (c *wsClient) close() {
	c.once.Do(func() {
		close(c.done)
		_ = c.conn.Close()

		c.mu.Lock()
		subscriptions := c.subscriptions
		c.subscriptions = make(map[string]*stream.Subscription)
		c.mu.Unlock()
		for _, subscription := range subscriptions {
			subscription.Close()
		}
	})
}

func websocketOriginCheck(allowed []string) func(*http.Request) bool {
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

	return func(request *http.Request) bool {
		origin := request.Header.Get("Origin")
		if origin == "" {
			return true
		}
		if allowAny {
			return true
		}
		if _, ok := allowedOrigins[origin]; ok {
			return true
		}
		parsed, err := url.Parse(origin)
		if err != nil {
			return false
		}
		return strings.EqualFold(parsed.Host, request.Host)
	}
}
