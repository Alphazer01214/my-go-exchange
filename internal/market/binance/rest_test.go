package binance

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"my-go-exchange/internal/marketdata"
	"time"
)

func TestFetchDepthParsesAndSortsSpotBook(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v3/depth" {
			t.Errorf("path = %q, want /api/v3/depth", r.URL.Path)
		}
		if got := r.URL.Query().Get("symbol"); got != "BTCUSDT" {
			t.Errorf("symbol = %q, want BTCUSDT", got)
		}
		if got := r.URL.Query().Get("limit"); got != "5" {
			t.Errorf("limit = %q, want 5", got)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"lastUpdateId": 42,
			"bids": [["101.00000000", "2.50000000"], ["102.00000000", "1.00000000"], ["bad", "1"]],
			"asks": [["104.00000000", "3.00000000"], ["103.50000000", "4.00000000"]]
		}`))
	}))
	defer server.Close()

	src := New("BTC/USDT")
	src.Base = server.URL
	src.Levels = 5
	book, err := src.FetchDepth(context.Background())
	if err != nil {
		t.Fatalf("FetchDepth: %v", err)
	}
	if book.Symbol != "btcusdt" {
		t.Fatalf("symbol = %q, want btcusdt", book.Symbol)
	}
	if book.LastUpdateID != 42 {
		t.Fatalf("lastUpdateID = %d, want 42", book.LastUpdateID)
	}
	if len(book.Bids) != 2 || book.Bids[0].Price != 10_200_000_000 || book.Bids[1].Price != 10_100_000_000 {
		t.Fatalf("bids = %+v, want descending valid levels", book.Bids)
	}
	if len(book.Asks) != 2 || book.Asks[0].Price != 10_350_000_000 || book.Asks[1].Price != 10_400_000_000 {
		t.Fatalf("asks = %+v, want ascending valid levels", book.Asks)
	}
	if book.Asks[0].Amount != 400_000_000 {
		t.Fatalf("ask amount = %d, want 400000000", book.Asks[0].Amount)
	}
	if book.Timestamp <= 0 {
		t.Fatalf("timestamp = %d, want milliseconds", book.Timestamp)
	}
}

func TestFetchDepthErrors(t *testing.T) {
	t.Run("http status", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			http.Error(w, `{"code":-1121,"msg":"invalid symbol"}`, http.StatusBadRequest)
		}))
		defer server.Close()
		src := New("BTCUSDT")
		src.Base = server.URL
		_, err := src.FetchDepth(context.Background())
		if err == nil || !strings.Contains(err.Error(), "-1121") {
			t.Fatalf("error = %v, want Binance error body", err)
		}
	})

	t.Run("bad json", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			_, _ = w.Write([]byte("{"))
		}))
		defer server.Close()
		src := New("BTCUSDT")
		src.Base = server.URL
		if _, err := src.FetchDepth(context.Background()); err == nil {
			t.Fatal("expected decode error")
		}
	})

	t.Run("empty side", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			_, _ = w.Write([]byte(`{"lastUpdateId":1,"bids":[],"asks":[["1","1"]]}`))
		}))
		defer server.Close()
		src := New("BTCUSDT")
		src.Base = server.URL
		if _, err := src.FetchDepth(context.Background()); err == nil {
			t.Fatal("expected empty-side error")
		}
	})
}

func TestAPISymbolNormalization(t *testing.T) {
	if got := apiSymbol(" btc/usdt "); got != "BTCUSDT" {
		t.Fatalf("apiSymbol = %q, want BTCUSDT", got)
	}
}

func TestRunRetriesAndStopsOnCancellation(t *testing.T) {
	var requests int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requests++
		if requests == 1 {
			http.Error(w, "temporary", http.StatusBadGateway)
			return
		}
		_, _ = w.Write([]byte(`{"lastUpdateId":7,"bids":[["10","1"]],"asks":[["11","1"]]}`))
	}))
	defer server.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	src := New("BTCUSDT")
	src.Base = server.URL
	src.Interval = time.Millisecond
	out := make(chan marketdata.Book, 1)
	done := make(chan error, 1)
	go func() { done <- src.Run(ctx, out) }()

	select {
	case book := <-out:
		if book.LastUpdateID != 7 {
			t.Fatalf("lastUpdateID = %d, want 7", book.LastUpdateID)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for retried book")
	}
	cancel()
	if err := <-done; err != nil {
		t.Fatalf("Run after cancellation: %v", err)
	}
	if requests < 2 {
		t.Fatalf("requests = %d, want retry after transient error", requests)
	}
}
