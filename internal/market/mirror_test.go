package market

import (
	"my-go-exchange/internal/trading"
	"testing"

	"my-go-exchange/internal/constant"
	"my-go-exchange/internal/matching"
)

const testScale = int64(100_000_000)

func newMirrorFixture() (*Mirror, *matching.Engine) {
	engine := matching.NewEngine(&trading.Instrument{
		Symbol: "BTC/USDT",
		Base:   "BTC",
		Quote:  "USDT",
	}, matching.NewIDAllocator())
	return NewMirror(engine, 1001), engine
}

func testBook(updateID int64, bids, asks []L2) L2Book {
	return L2Book{
		Symbol:       "BTC/USDT",
		Bids:         bids,
		Asks:         asks,
		LastUpdateID: updateID,
		Timestamp:    updateID,
	}
}

func TestMirrorApplyBuildsContractStyleBook(t *testing.T) {
	mirror, engine := newMirrorFixture()
	events := mirror.Apply(testBook(1,
		[]L2{{Price: 100 * testScale, Amount: 10 * testScale}},
		[]L2{{Price: 101 * testScale, Amount: 8 * testScale}},
	))
	if len(events) != 2 {
		t.Fatalf("events = %d, want two accepted mirror orders", len(events))
	}

	bid, bidAmount, ok := engine.OrderBook().BestBid()
	if !ok || bid != 100*testScale || bidAmount != 10*testScale {
		t.Fatalf("best bid = %d/%d/%v", bid, bidAmount, ok)
	}
	ask, askAmount, ok := engine.OrderBook().BestAsk()
	if !ok || ask != 101*testScale || askAmount != 8*testScale {
		t.Fatalf("best ask = %d/%d/%v", ask, askAmount, ok)
	}

	for _, orderID := range []int64{mirror.bids[100*testScale], mirror.asks[101*testScale]} {
		order := engine.GetOrder(orderID)
		if order == nil || order.Leverage != 1 || order.Type != constant.Limit {
			t.Fatalf("mirror order = %+v, want limit leverage 1", order)
		}
	}
	if engine.GetOrder(mirror.bids[100*testScale]).PositionSide != constant.Long {
		t.Fatal("mirror bid should be long")
	}
	if engine.GetOrder(mirror.asks[101*testScale]).PositionSide != constant.Short {
		t.Fatal("mirror ask should be short")
	}
}

func TestMirrorApplyWithResultAggregatesBookChanges(t *testing.T) {
	mirror, _ := newMirrorFixture()
	result := mirror.ApplyWithResult(testBook(1,
		[]L2{{Price: 100 * testScale, Amount: 10 * testScale}},
		[]L2{{Price: 101 * testScale, Amount: 8 * testScale}},
	))
	if len(result.BookChanges.Bids) != 1 || len(result.BookChanges.Asks) != 1 {
		t.Fatalf("book changes = %+v, want one bid and one ask", result.BookChanges)
	}
	if result.BookChanges.Bids[0].Price != 100*testScale ||
		result.BookChanges.Asks[0].Price != 101*testScale {
		t.Fatalf("book changes = %+v", result.BookChanges)
	}
}

func TestMirrorApplyIsIdempotentForUnchangedBook(t *testing.T) {
	mirror, _ := newMirrorFixture()
	book := testBook(1,
		[]L2{{Price: 100 * testScale, Amount: 10 * testScale}},
		[]L2{{Price: 101 * testScale, Amount: 10 * testScale}},
	)
	mirror.Apply(book)
	if events := mirror.Apply(book); len(events) != 0 {
		t.Fatalf("unchanged Apply events = %+v, want none", events)
	}
}

func TestMirrorApplyReplacesChangedAndStaleLevels(t *testing.T) {
	mirror, engine := newMirrorFixture()
	mirror.Apply(testBook(1,
		[]L2{
			{Price: 100 * testScale, Amount: 10 * testScale},
			{Price: 99 * testScale, Amount: 5 * testScale},
		},
		[]L2{{Price: 101 * testScale, Amount: 10 * testScale}},
	))

	mirror.Apply(testBook(2,
		[]L2{{Price: 100 * testScale, Amount: 12 * testScale}},
		[]L2{{Price: 102 * testScale, Amount: 7 * testScale}},
	))

	if got := len(engine.OrderBook().Bids(10)); got != 1 {
		t.Fatalf("bid levels = %d, want 1", got)
	}
	if got := len(engine.OrderBook().Asks(10)); got != 1 {
		t.Fatalf("ask levels = %d, want 1", got)
	}
	bid, bidAmount, _ := engine.OrderBook().BestBid()
	ask, askAmount, _ := engine.OrderBook().BestAsk()
	if bid != 100*testScale || bidAmount != 12*testScale {
		t.Fatalf("best bid = %d/%d", bid, bidAmount)
	}
	if ask != 102*testScale || askAmount != 7*testScale {
		t.Fatalf("best ask = %d/%d", ask, askAmount)
	}
}

func TestMirrorRestoresLevelConsumedByUserOrder(t *testing.T) {
	mirror, engine := newMirrorFixture()
	book := testBook(1,
		[]L2{{Price: 100 * testScale, Amount: 10 * testScale}},
		[]L2{{Price: 101 * testScale, Amount: 10 * testScale}},
	)
	mirror.Apply(book)

	engine.Apply(&matching.PlaceOrder{
		AccountID:    2002,
		Price:        101 * testScale,
		Amount:       4 * testScale,
		OrderSide:    constant.Buy,
		PositionSide: constant.Long,
		Type:         constant.Limit,
		Leverage:     1,
	})
	if _, amount, _ := engine.OrderBook().BestAsk(); amount != 6*testScale {
		t.Fatalf("ask after user fill = %d, want %d", amount, 6*testScale)
	}

	book.LastUpdateID = 2
	book.Timestamp = 2
	mirror.Apply(book)
	if _, amount, _ := engine.OrderBook().BestAsk(); amount != 10*testScale {
		t.Fatalf("restored ask = %d, want %d", amount, 10*testScale)
	}
}

func TestMirrorRejectsInvalidOrStaleSnapshotsAtomically(t *testing.T) {
	mirror, engine := newMirrorFixture()
	valid := testBook(10,
		[]L2{{Price: 100 * testScale, Amount: 1 * testScale}},
		[]L2{{Price: 101 * testScale, Amount: 1 * testScale}},
	)
	mirror.Apply(valid)

	crossed := testBook(11,
		[]L2{{Price: 102 * testScale, Amount: 2 * testScale}},
		[]L2{{Price: 101 * testScale, Amount: 2 * testScale}},
	)
	if events := mirror.Apply(crossed); len(events) != 0 {
		t.Fatalf("crossed snapshot events = %+v, want none", events)
	}

	stale := testBook(9,
		[]L2{{Price: 90 * testScale, Amount: 9 * testScale}},
		[]L2{{Price: 91 * testScale, Amount: 9 * testScale}},
	)
	if events := mirror.Apply(stale); len(events) != 0 {
		t.Fatalf("stale snapshot events = %+v, want none", events)
	}

	if bid, amount, _ := engine.OrderBook().BestBid(); bid != 100*testScale || amount != testScale {
		t.Fatalf("bid changed after invalid snapshots: %d/%d", bid, amount)
	}
	if ask, amount, _ := engine.OrderBook().BestAsk(); ask != 101*testScale || amount != testScale {
		t.Fatalf("ask changed after invalid snapshots: %d/%d", ask, amount)
	}
}
