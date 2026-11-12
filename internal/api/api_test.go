package api_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"my-go-exchange/internal/api"
	"my-go-exchange/internal/market"
	"my-go-exchange/internal/marketdata"
	"my-go-exchange/internal/trading"
)

func TestAccountMarketAndOrderEndpoints(t *testing.T) {
	exchange := newAPIMarket(t)
	server := api.NewServer(exchange, testAPIConfig(t))

	response := request(t, server, http.MethodPost, "/api/account/deposit", map[string]any{
		"currency": "USDT",
		"amount":   "1000",
	})
	assertStatus(t, response, http.StatusOK)

	response = request(t, server, http.MethodGet, "/api/instruments", nil)
	assertStatus(t, response, http.StatusOK)

	response = request(t, server, http.MethodGet, "/api/book?symbol=BTC%2FUSDT&depth=1", nil)
	assertStatus(t, response, http.StatusOK)

	response = request(t, server, http.MethodPost, "/api/orders", map[string]any{
		"account_id":      2,
		"client_order_id": "api-open-long",
		"type":            "limit",
		"symbol":          "BTC/USDT",
		"order_side":      "buy",
		"position_side":   "long",
		"price":           "101",
		"amount":          "2",
		"leverage":        1,
	})
	assertStatus(t, response, http.StatusOK)
	var opened api.APIResponse
	decodeResponse(t, response, &opened)
	orderData := opened.Data.(map[string]any)
	orderID := int64(orderData["id"].(float64))
	if orderData["account_id"] != float64(1) {
		t.Fatalf("order account = %+v, want authenticated account 1", orderData["account_id"])
	}
	if orderID <= 0 || orderData["state"] != "filled" {
		t.Fatalf("opened order = %+v", orderData)
	}
	if _, ok := orderData["price"].(string); !ok {
		t.Fatalf("order price = %#v, want fixed-point string", orderData["price"])
	}

	response = request(t, server, http.MethodGet, "/api/account/balances", nil)
	assertStatus(t, response, http.StatusOK)
	var balances api.APIResponse
	decodeResponse(t, response, &balances)
	balanceData := balances.Data.([]any)[0].(map[string]any)
	if balanceData["available"] != "798" {
		t.Fatalf("balance = %+v, want available 798", balanceData)
	}

	response = request(t, server, http.MethodGet, "/api/account/positions", nil)
	assertStatus(t, response, http.StatusOK)
	var positions api.APIResponse
	decodeResponse(t, response, &positions)
	positionData := positions.Data.([]any)[0].(map[string]any)
	if positionData["symbol"] != "BTC/USDT" || positionData["position_side"] != "long" {
		t.Fatalf("position = %+v", positionData)
	}

	response = request(t, server, http.MethodGet, "/api/orders/"+jsonNumber(orderID), nil)
	assertStatus(t, response, http.StatusOK)

	response = request(t, server, http.MethodPost, "/api/orders", map[string]any{
		"client_order_id": "api-close-long",
		"type":            "limit",
		"symbol":          "BTC/USDT",
		"order_side":      "sell",
		"position_side":   "long",
		"price":           "102",
		"amount":          "1",
		"leverage":        1,
	})
	assertStatus(t, response, http.StatusOK)
	var closeOrder api.APIResponse
	decodeResponse(t, response, &closeOrder)
	closeOrderData := closeOrder.Data.(map[string]any)
	closeOrderID := int64(closeOrderData["id"].(float64))

	response = request(
		t,
		server,
		http.MethodDelete,
		"/api/orders/"+jsonNumber(closeOrderID),
		nil,
	)
	assertStatus(t, response, http.StatusOK)
	var canceled api.APIResponse
	decodeResponse(t, response, &canceled)
	if canceled.Data.(map[string]any)["state"] != "canceled" {
		t.Fatalf("canceled order = %+v", canceled.Data)
	}

}

func TestAPIErrorAndCORSResponse(t *testing.T) {
	exchange := newAPIMarket(t)
	server := api.NewServer(exchange, testAPIConfig(t))

	response := request(t, server, http.MethodGet, "/api/account/balances", nil)
	assertStatus(t, response, http.StatusOK)

	response = request(t, server, http.MethodPost, "/api/orders", map[string]any{
		"client_order_id": "insufficient",
		"type":            "limit",
		"symbol":          "BTC/USDT",
		"order_side":      "buy",
		"position_side":   "long",
		"price":           101,
		"amount":          100,
		"leverage":        1,
	})
	assertStatus(t, response, http.StatusConflict)

	response = request(t, server, http.MethodOptions, "/api/orders", nil)
	if response.Code != http.StatusNoContent {
		t.Fatalf("OPTIONS status = %d, want 204", response.Code)
	}
	if response.Header().Get("Access-Control-Allow-Origin") != "*" {
		t.Fatal("missing CORS allow-origin header")
	}
}

func TestProtectedEndpointsRequireAuthentication(t *testing.T) {
	exchange := newAPIMarket(t)
	server := api.NewServer(exchange, testAPIConfig(t))

	response := requestWithoutAuth(t, server, http.MethodGet, "/api/account/balances", nil)
	assertStatus(t, response, http.StatusUnauthorized)
	if response.Header().Get("WWW-Authenticate") == "" {
		t.Fatal("missing WWW-Authenticate header")
	}

	response = requestWithToken(
		t,
		server,
		http.MethodGet,
		"/api/account/balances",
		nil,
		"wrong-token",
	)
	assertStatus(t, response, http.StatusUnauthorized)

	response = requestWithoutAuth(t, server, http.MethodGet, "/api/instruments", nil)
	assertStatus(t, response, http.StatusOK)
}

func newAPIMarket(t *testing.T) *market.Market {
	t.Helper()
	exchange := market.NewMarket()
	local, err := exchange.RegisterLocal(&trading.Instrument{
		Symbol: "BTC/USDT",
		Base:   "BTC",
		Quote:  "USDT",
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := local.ApplyBook(context.Background(), marketdata.Book{
		Symbol: "BTC/USDT",
		Bids:   []marketdata.Level{{Price: 100, Amount: 10}},
		Asks:   []marketdata.Level{{Price: 101, Amount: 10}},
	}); err != nil {
		t.Fatal(err)
	}
	return exchange
}

const testAPIToken = "test-token"

func testAPIConfig(t *testing.T) api.Config {
	t.Helper()
	authenticator, err := api.NewStaticTokenAuthenticator(testAPIToken, 1)
	if err != nil {
		t.Fatal(err)
	}
	return api.Config{
		Production:    true,
		Authenticator: authenticator,
	}
}

func request(t *testing.T, server *api.Server, method, path string, body any) *httptest.ResponseRecorder {
	t.Helper()
	return requestWithToken(t, server, method, path, body, testAPIToken)
}

func requestWithoutAuth(
	t *testing.T,
	server *api.Server,
	method, path string,
	body any,
) *httptest.ResponseRecorder {
	t.Helper()
	return requestWithToken(t, server, method, path, body, "")
}

func requestWithToken(
	t *testing.T,
	server *api.Server,
	method, path string,
	body any,
	token string,
) *httptest.ResponseRecorder {
	t.Helper()
	var payload []byte
	var err error
	if body != nil {
		payload, err = json.Marshal(body)
		if err != nil {
			t.Fatal(err)
		}
	}
	req := httptest.NewRequest(method, path, bytes.NewReader(payload))
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	response := httptest.NewRecorder()
	server.Handler().ServeHTTP(response, req)
	return response
}

func decodeResponse(t *testing.T, response *httptest.ResponseRecorder, target any) {
	t.Helper()
	if err := json.Unmarshal(response.Body.Bytes(), target); err != nil {
		t.Fatalf("decode response %q: %v", response.Body.String(), err)
	}
}

func assertStatus(t *testing.T, response *httptest.ResponseRecorder, want int) {
	t.Helper()
	if response.Code != want {
		t.Fatalf("status = %d, want %d; body = %s", response.Code, want, response.Body.String())
	}
}

func jsonNumber(value int64) string {
	return strconv.FormatInt(value, 10)
}
