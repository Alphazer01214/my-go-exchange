package gate

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"my-go-exchange/internal/marketdata"
)

func TestFetchOrderBookParsesTokenizedEquity(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/spot/order_book" {
			t.Errorf("path = %q, want /spot/order_book", r.URL.Path)
		}
		if got := r.URL.Query().Get("currency_pair"); got != "NVDAON_USDT" {
			t.Errorf("currency_pair = %q", got)
		}
		_, _ = w.Write([]byte(`{
			"id": 166820609,
			"current": 1794491488553,
			"asks": [["157.63","0.079"],["157.64","0.089"]],
			"bids": [["156.61","0.017"],["156.00","0.001"]]
		}`))
	}))
	defer server.Close()

	source := NewSourceFor("NVDAON_USDT")
	source.Base = server.URL
	source.HTTP = server.Client()

	book, err := source.FetchOrderBook(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if book.Symbol != "NVDAON_USDT" || book.LastUpdateID != 166820609 {
		t.Fatalf("book identity = %+v", book)
	}
	if len(book.Bids) != 2 || len(book.Asks) != 2 {
		t.Fatalf("levels = %d bids / %d asks", len(book.Bids), len(book.Asks))
	}
	if book.Bids[0].Price != 15_661_000_000 || book.Asks[0].Price != 15_763_000_000 {
		t.Fatalf("best prices = %d / %d", book.Bids[0].Price, book.Asks[0].Price)
	}
	if book.Timestamp != 1794491488553 {
		t.Fatalf("timestamp = %d", book.Timestamp)
	}
}

func TestRunPublishesGateBook(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{
			"id": 7,
			"current": 1794491488553,
			"asks": [["11","1"]],
			"bids": [["10","1"]]
		}`))
	}))
	defer server.Close()

	source := NewSourceFor("NVDAON_USDT")
	source.Base = server.URL
	source.HTTP = server.Client()
	source.Interval = time.Millisecond

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	out := make(chan marketdata.Book, 1)
	done := make(chan error, 1)
	go func() { done <- source.Run(ctx, out) }()

	select {
	case book := <-out:
		if book.LastUpdateID != 7 {
			t.Fatalf("last update = %d", book.LastUpdateID)
		}
	case <-ctx.Done():
		t.Fatal("timed out waiting for Gate book")
	}
	cancel()
	if err := <-done; err != nil {
		t.Fatalf("Run: %v", err)
	}
}
