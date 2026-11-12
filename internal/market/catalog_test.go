package market_test

import (
	"context"
	"testing"
	"time"

	"my-go-exchange/internal/market"
)

func TestDefaultMarketLoadsCryptoAndNasdaqSymbols(t *testing.T) {
	exchange, err := market.NewDefaultMarket()
	if err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	if err := exchange.Start(ctx); err != nil {
		t.Fatal(err)
	}
}

func TestDefaultTokenizedInstruments(t *testing.T) {
	instruments := market.DefaultTokenizedInstruments()
	if len(instruments) != 9 {
		t.Fatalf("tokenized instruments = %d, want 9", len(instruments))
	}
	if instruments[2].Symbol != "NVDA/USDT" {
		t.Fatalf("NVDA tokenized symbol = %q", instruments[2].Symbol)
	}
}

func TestLocalMarketProducesBook(t *testing.T) {
	exchange, err := market.NewLocalMarket()
	if err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	if err := exchange.Start(ctx); err != nil {
		t.Fatal(err)
	}

	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		book, err := exchange.Book(ctx, "BTC/USDT")
		if err == nil && len(book.Bids) > 0 && len(book.Asks) > 0 {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatal("local source did not produce a book")
}
