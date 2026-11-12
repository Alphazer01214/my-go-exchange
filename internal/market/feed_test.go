package market

import (
	"context"
	"my-go-exchange/internal/trading"
	"net/http"
	"net/http/httptest"
	"testing"

	"my-go-exchange/internal/market/binance"
)

var _ L2Feed = (*binance.Source)(nil)

func TestBinanceSymbolNormalization(t *testing.T) {
	if got := binanceSymbol(" btc/usdt "); got != "BTCUSDT" {
		t.Fatalf("binanceSymbol = %q, want BTCUSDT", got)
	}
}

func TestUseBinanceRouting(t *testing.T) {
	tests := []struct {
		name   string
		market *LocalSymbolMarket
		want   bool
	}{
		{
			name:   "local crypto pair",
			market: &LocalSymbolMarket{symbol: "BTC/USDT", inst: &trading.Instrument{Base: "BTC", Quote: "USDT"}},
			want:   true,
		},
		{
			name:   "compact crypto pair",
			market: &LocalSymbolMarket{symbol: "BTCUSDT", inst: &trading.Instrument{Base: "BTC", Quote: "USDT"}},
			want:   true,
		},
		{
			name:   "nasdaq stock",
			market: &LocalSymbolMarket{symbol: "AAPL", inst: &trading.Instrument{Base: "AAPL", Quote: "USD"}},
			want:   false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.market.useBinance(); got != tt.want {
				t.Fatalf("useBinance() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestNasdaqPoolBuildsMirroredBook(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{
			"data": {
				"symbol": "AAPL",
				"primaryData": {
					"lastSalePrice": "101",
					"bidPrice": "100.99",
					"askPrice": "101.01",
					"volume": "100"
				},
				"secondaryData": {
					"previousClose": "100",
					"todaysHigh": "102",
					"todaysLow": "99",
					"open": "100"
				}
			}
		}`))
	}))
	defer server.Close()

	src := newTestNasdaqSource(server.URL)
	m := &LocalSymbolMarket{symbol: "AAPL", inst: &trading.Instrument{Symbol: "AAPL", Base: "AAPL", Quote: "USD"}}
	out := make(chan L2Book, 1)
	if err := m.nasdaqPool(context.Background(), src, out); err != nil {
		t.Fatalf("nasdaqPool: %v", err)
	}
	book := <-out
	if book.Symbol != "AAPL" {
		t.Fatalf("symbol = %q, want AAPL", book.Symbol)
	}
	if _, _, ok := book.Best(); !ok {
		t.Fatal("expected valid best bid/ask")
	}
}
