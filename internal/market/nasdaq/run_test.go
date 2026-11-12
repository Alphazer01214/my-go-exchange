package nasdaq

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"my-go-exchange/internal/marketdata"
)

func TestRunPublishesSyntheticBook(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{
			"data": {
				"symbol": "AAPL",
				"primaryData": {
					"lastSalePrice": "$101.25",
					"bidPrice": "101.20",
					"askPrice": "101.30"
				},
				"secondaryData": {}
			}
		}`))
	}))
	defer server.Close()

	src := NewSourceFor("AAPL")
	src.Base = server.URL
	src.Http = server.Client()
	src.Interval = time.Millisecond

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	out := make(chan marketdata.Book, 1)
	done := make(chan error, 1)
	go func() { done <- src.Run(ctx, out) }()

	select {
	case book := <-out:
		if book.Symbol != "AAPL" || len(book.Bids) != 1 || len(book.Asks) != 1 {
			t.Fatalf("book = %+v, want synthetic AAPL top-of-book", book)
		}
	case <-ctx.Done():
		t.Fatal("timed out waiting for Nasdaq book")
	}
	cancel()
	if err := <-done; err != nil {
		t.Fatalf("Run: %v", err)
	}
}
