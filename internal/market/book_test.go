package market

import "testing"

func TestBuildFromQuoteUsesVisibleSpreadAsStep(t *testing.T) {
	book := BuildFromQuote("BTC/USDT", 100, 101, 100.5, 2, 10)
	if len(book.Bids) != 2 || len(book.Asks) != 2 {
		t.Fatalf("levels = %d bids / %d asks, want 2 / 2", len(book.Bids), len(book.Asks))
	}

	wantBids := []L2{{Price: 10_000_000_000, Amount: 1_000_000_000}, {Price: 9_900_000_000, Amount: 1_000_000_000}}
	wantAsks := []L2{{Price: 10_100_000_000, Amount: 1_000_000_000}, {Price: 10_200_000_000, Amount: 1_000_000_000}}
	for i := range wantBids {
		if book.Bids[i] != wantBids[i] {
			t.Fatalf("bid[%d] = %+v, want %+v", i, book.Bids[i], wantBids[i])
		}
		if book.Asks[i] != wantAsks[i] {
			t.Fatalf("ask[%d] = %+v, want %+v", i, book.Asks[i], wantAsks[i])
		}
	}
	if book.Timestamp <= 0 {
		t.Fatalf("timestamp = %d, want milliseconds", book.Timestamp)
	}
}

func TestBuildFromQuoteBuildsPositiveSpreadForCheapInstrument(t *testing.T) {
	book := BuildFromQuote("PENNY", 0, 0, 0.01, 3, 1)
	bid, ask, ok := book.Best()
	if !ok {
		t.Fatal("expected synthetic best bid/ask")
	}
	if bid.Price <= 0 || ask.Price <= bid.Price {
		t.Fatalf("best = %+v / %+v, want positive non-crossed spread", bid, ask)
	}
	if len(book.Bids) != 3 || len(book.Asks) != 3 {
		t.Fatalf("levels = %d / %d, want 3 / 3", len(book.Bids), len(book.Asks))
	}
}

func TestBuildBookRejectsInvalidConfig(t *testing.T) {
	for _, cfg := range []Config{
		{},
		{Mid: 100, Spread: 1, Depth: 1, Levels: 0},
		{Mid: 100, Spread: 0, Depth: 1, Levels: 1},
		{Mid: 100, Spread: 1, Depth: 0, Levels: 1},
	} {
		book := BuildBook("TEST", cfg)
		if _, _, ok := book.Best(); ok {
			t.Fatalf("BuildBook(%+v) unexpectedly produced a book", cfg)
		}
	}
}
