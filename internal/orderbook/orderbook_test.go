package orderbook

import (
	"testing"

	"my-go-exchange/internal/types"
)

func newBook() *OrderBook {
	return NewOrderBook(&types.Instrument{Symbol: "BTC/USDT"})
}

func mkOrder(id uint64, side types.Side, price, amount uint64) *types.Order {
	return &types.Order{
		ID:        id,
		Side:      side,
		Type:      types.Limit,
		Price:     price,
		Amount:    amount,
		Remaining: amount,
		TIF:       types.GTC,
	}
}

func TestPlaceAndBest(t *testing.T) {
	ob := newBook()
	if err := ob.Place(mkOrder(1, types.Buy, 100, 10)); err != nil {
		t.Fatal(err)
	}
	if err := ob.Place(mkOrder(2, types.Sell, 101, 5)); err != nil {
		t.Fatal(err)
	}

	p, a, ok := ob.BestBid()
	if !ok || p != 100 || a != 10 {
		t.Fatalf("BestBid = (%d,%d,%v)", p, a, ok)
	}
	p, a, ok = ob.BestAsk()
	if !ok || p != 101 || a != 5 {
		t.Fatalf("BestAsk = (%d,%d,%v)", p, a, ok)
	}
}

func TestBidPriceOrder(t *testing.T) {
	ob := newBook()
	for _, o := range []*types.Order{
		mkOrder(1, types.Buy, 100, 1),
		mkOrder(2, types.Buy, 102, 1),
		mkOrder(3, types.Buy, 101, 1),
	} {
		if err := ob.Place(o); err != nil {
			t.Fatal(err)
		}
	}
	levels := ob.Bids(3)
	want := []uint64{102, 101, 100}
	for i, w := range want {
		if levels[i].Price != w {
			t.Fatalf("bids[%d]=%d, want %d", i, levels[i].Price, w)
		}
	}
}

func TestFillPartialThenFull(t *testing.T) {
	ob := newBook()
	if err := ob.Place(mkOrder(1, types.Sell, 100, 10)); err != nil {
		t.Fatal(err)
	}

	if err := ob.Fill(1, 4); err != nil {
		t.Fatal(err)
	}
	_, a, ok := ob.BestAsk()
	if !ok || a != 6 {
		t.Fatalf("after partial fill ask=%d, want 6", a)
	}

	if err := ob.Fill(1, 6); err != nil {
		t.Fatal(err)
	}
	if _, _, ok := ob.BestAsk(); ok {
		t.Fatalf("ask should be empty after full fill")
	}
	if _, err := ob.Cancel(1); err == nil {
		t.Fatalf("filled order should not be cancellable")
	}
}

func TestCancel(t *testing.T) {
	ob := newBook()
	if err := ob.Place(mkOrder(1, types.Buy, 100, 10)); err != nil {
		t.Fatal(err)
	}
	order, err := ob.Cancel(1)
	if err != nil {
		t.Fatal(err)
	}
	if order.State != types.StateCanceled {
		t.Fatalf("state = %v", order.State)
	}
	if _, _, ok := ob.BestBid(); ok {
		t.Fatalf("bid should be empty")
	}
}

func TestMarketOrderCannotRest(t *testing.T) {
	ob := newBook()
	o := mkOrder(1, types.Buy, 0, 10)
	o.Type = types.Market
	if err := ob.Place(o); err == nil {
		t.Fatalf("market order must not rest")
	}
}

func TestDuplicateID(t *testing.T) {
	ob := newBook()
	if err := ob.Place(mkOrder(1, types.Buy, 100, 1)); err != nil {
		t.Fatal(err)
	}
	if err := ob.Place(mkOrder(1, types.Sell, 101, 1)); err == nil {
		t.Fatalf("duplicate id should fail")
	}
}

func TestAvailableForFOK(t *testing.T) {
	ob := newBook()
	_ = ob.Place(mkOrder(1, types.Sell, 100, 2))
	_ = ob.Place(mkOrder(2, types.Sell, 105, 3))
	_ = ob.Place(mkOrder(3, types.Sell, 110, 100))

	// buy limit 105: 可吃 2+3=5
	if got := ob.Available(types.Buy, 105, false); got != 5 {
		t.Fatalf("Available buy@105 = %d, want 5", got)
	}
	// market buy: 全部 105
	if got := ob.Available(types.Buy, 0, true); got != 105 {
		t.Fatalf("Available market buy = %d, want 105", got)
	}
}
