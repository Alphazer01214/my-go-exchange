package api_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"my-go-exchange/internal/api"
	"my-go-exchange/internal/constant"
	"my-go-exchange/internal/market"

	"github.com/gorilla/websocket"
)

type testAuthenticator struct {
	accounts map[string]int64
}

func (a testAuthenticator) Authenticate(_ context.Context, token string) (int64, error) {
	accountID, ok := a.accounts[token]
	if !ok {
		return 0, errors.New("invalid token")
	}
	return accountID, nil
}

type wsMessage struct {
	Type      string          `json:"type"`
	RequestID string          `json:"request_id"`
	Stream    string          `json:"stream"`
	Seq       int64           `json:"seq"`
	Data      json.RawMessage `json:"data"`
	Error     *api.APIError   `json:"error"`
}

func TestWebSocketPushesBookTradeAndPrivateOrderEvents(t *testing.T) {
	exchange := newAPIMarket(t)
	if err := exchange.Deposit(context.Background(), 1, "USDT", 1000); err != nil {
		t.Fatal(err)
	}
	server := api.NewServer(exchange, api.Config{
		Production: true,
		Authenticator: testAuthenticator{accounts: map[string]int64{
			"account-1": 1,
		}},
	})
	httpServer := httptest.NewServer(server.Handler())
	defer httpServer.Close()

	conn := dialWebSocket(t, httpServer)
	defer conn.Close()

	writeWS(t, conn, map[string]any{
		"op":         "auth",
		"request_id": "auth-1",
		"token":      "account-1",
	})
	readUntilWS(t, conn, func(message wsMessage) bool {
		return message.Type == "ack" && message.RequestID == "auth-1"
	})

	writeWS(t, conn, map[string]any{
		"op":         "subscribe",
		"request_id": "sub-1",
		"streams": []map[string]any{
			{"name": "book.BTC/USDT", "depth": 5},
			{"name": "trades.BTC/USDT"},
			{"name": "orders"},
		},
	})
	readUntilWS(t, conn, func(message wsMessage) bool {
		return message.Type == "ack" && message.RequestID == "sub-1"
	})

	if _, err := exchange.Place(context.Background(), market.PlaceReq{
		AccountID:     1,
		ClientOrderID: "ws-take",
		Type:          constant.Limit,
		Symbol:        "BTC/USDT",
		OrderSide:     constant.Buy,
		PositionSide:  constant.Long,
		Price:         101,
		Amount:        2,
		Leverage:      1,
	}); err != nil {
		t.Fatal(err)
	}

	seen := make(map[string]bool)
	deadline := time.Now().Add(2 * time.Second)
	for !(seen["book.delta"] && seen["trade"] && seen["order.update"] && seen["fill"]) {
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for events, saw %+v", seen)
		}
		message := readWS(t, conn)
		seen[message.Type] = true
	}
}

func TestWebSocketRejectsPrivateSubscriptionWithoutAuth(t *testing.T) {
	exchange := newAPIMarket(t)
	server := api.NewServer(exchange, api.Config{Production: true})
	httpServer := httptest.NewServer(server.Handler())
	defer httpServer.Close()

	conn := dialWebSocket(t, httpServer)
	defer conn.Close()
	writeWS(t, conn, map[string]any{
		"op":         "subscribe",
		"request_id": "sub-private",
		"streams":    []map[string]any{{"name": "orders"}},
	})

	message := readUntilWS(t, conn, func(message wsMessage) bool {
		return message.Type == "error" && message.RequestID == "sub-private"
	})
	if message.Error == nil || !strings.Contains(message.Error.Message, "authentication") {
		t.Fatalf("error = %+v", message.Error)
	}
}

func TestWebSocketRejectsAccountSwitchAfterAuthentication(t *testing.T) {
	exchange := newAPIMarket(t)
	server := api.NewServer(exchange, api.Config{
		Production: true,
		Authenticator: testAuthenticator{accounts: map[string]int64{
			"account-1": 1,
			"account-2": 2,
		}},
	})
	httpServer := httptest.NewServer(server.Handler())
	defer httpServer.Close()

	conn := dialWebSocket(t, httpServer)
	defer conn.Close()
	writeWS(t, conn, map[string]any{
		"op":         "auth",
		"request_id": "auth-1",
		"token":      "account-1",
	})
	readUntilWS(t, conn, func(message wsMessage) bool {
		return message.Type == "ack" && message.RequestID == "auth-1"
	})

	writeWS(t, conn, map[string]any{
		"op":         "auth",
		"request_id": "auth-2",
		"token":      "account-2",
	})
	message := readUntilWS(t, conn, func(message wsMessage) bool {
		return message.Type == "error" && message.RequestID == "auth-2"
	})
	if message.Error == nil || !strings.Contains(message.Error.Message, "already authenticated") {
		t.Fatalf("error = %+v", message.Error)
	}
}

func dialWebSocket(t *testing.T, server *httptest.Server) *websocket.Conn {
	t.Helper()
	url := "ws" + strings.TrimPrefix(server.URL, "http") + "/ws/v1"
	conn, _, err := websocket.DefaultDialer.Dial(url, nil)
	if err != nil {
		t.Fatal(err)
	}
	return conn
}

func writeWS(t *testing.T, conn *websocket.Conn, payload any) {
	t.Helper()
	if err := conn.WriteJSON(payload); err != nil {
		t.Fatal(err)
	}
}

func readUntilWS(
	t *testing.T,
	conn *websocket.Conn,
	match func(wsMessage) bool,
) wsMessage {
	t.Helper()
	for {
		message := readWS(t, conn)
		if match(message) {
			return message
		}
	}
}

func readWS(t *testing.T, conn *websocket.Conn) wsMessage {
	t.Helper()
	if err := conn.SetReadDeadline(time.Now().Add(2 * time.Second)); err != nil {
		t.Fatal(err)
	}
	_, payload, err := conn.ReadMessage()
	if err != nil {
		t.Fatal(err)
	}
	var message wsMessage
	if err := json.Unmarshal(payload, &message); err != nil {
		t.Fatalf("decode websocket message %q: %v", payload, err)
	}
	return message
}
