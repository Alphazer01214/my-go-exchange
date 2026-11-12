package orderbook

import (
	"errors"
	"my-go-exchange/internal/trading"
	"testing"

	"my-go-exchange/internal/constant"
)

func testBook() *OrderBook {
	return NewOrderBook(&trading.Instrument{Symbol: "TEST/QUOTE"})
}

func restingOrder(id int64, side constant.OrderSide, price, amount int64) *trading.Order {
	return &trading.Order{
		ID:        id,
		AccountID: 1,
		OrderSide: side,
		Type:      constant.Limit,
		Price:     price,
		Amount:    amount,
		Remaining: amount,
		// TIF:       constant.GTC,
		State: constant.StateAccept,
	}
}

func TestValidatePlace(t *testing.T) {
	tests := []struct {
		name      string
		orderType constant.OrderType
		price     int64
		amount    int64
		side      constant.OrderSide
		want      error
	}{
		{name: "valid limit", orderType: constant.Limit, price: 10, amount: 1, side: constant.Buy},
		{name: "valid market", orderType: constant.Market, amount: 1, side: constant.Sell},
		{name: "invalid side", orderType: constant.Limit, price: 10, amount: 1, side: constant.OrderSide("invalid"), want: constant.ErrInvalidSide},
		{name: "invalid type", orderType: constant.OrderType("invalid"), price: 10, amount: 1, side: constant.Buy, want: constant.ErrInvalidOrderType},
		// TIF validation is intentionally disabled for now.
		// {name: "invalid tif", orderType: constant.Limit, price: 10, amount: 1, side: constant.Buy, tif: constant.TIF(9), want: constant.ErrInvalidTIF},
		{name: "zero amount", orderType: constant.Limit, price: 10, side: constant.Buy, want: constant.ErrInvalidAmount},
		{name: "limit without price", orderType: constant.Limit, amount: 1, side: constant.Buy, want: constant.ErrInvalidPrice},
		{name: "market with price", orderType: constant.Market, price: 10, amount: 1, side: constant.Buy, want: constant.ErrMarketOrderWithPrice},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := ValidatePlace(test.orderType, test.price, test.amount, test.side)
			if !errors.Is(err, test.want) {
				t.Fatalf("error = %v, want %v", err, test.want)
			}
		})
	}
}

func TestPlaceMaintainsPricePriority(t *testing.T) {
	book := testBook()
	for _, order := range []*trading.Order{
		restingOrder(1, constant.Buy, 100, 1),
		restingOrder(2, constant.Buy, 102, 2),
		restingOrder(3, constant.Buy, 101, 3),
		restingOrder(4, constant.Sell, 105, 4),
		restingOrder(5, constant.Sell, 103, 5),
	} {
		if err := book.Rest(order); err != nil {
			t.Fatalf("Rest(%d): %v", order.ID, err)
		}
	}

	bids := book.Bids(3)
	wantBids := []Level{
		{Price: 102, Amount: 2, OrderCount: 1},
		{Price: 101, Amount: 3, OrderCount: 1},
		{Price: 100, Amount: 1, OrderCount: 1},
	}
	assertLevels(t, bids, wantBids)

	asks := book.Asks(2)
	wantAsks := []Level{
		{Price: 103, Amount: 5, OrderCount: 1},
		{Price: 105, Amount: 4, OrderCount: 1},
	}
	assertLevels(t, asks, wantAsks)
}

func TestSamePriceUsesTimePriority(t *testing.T) {
	book := testBook()
	first := restingOrder(1, constant.Sell, 100, 2)
	second := restingOrder(2, constant.Sell, 100, 3)
	if err := book.Rest(first); err != nil {
		t.Fatal(err)
	}
	if err := book.Rest(second); err != nil {
		t.Fatal(err)
	}

	got, ok := book.Front(constant.Sell)
	if !ok || got.ID != 1 {
		t.Fatalf("Front = (%+v,%v), want order 1", got, ok)
	}
	if got != first {
		t.Fatal("Front should return the resting order pointer")
	}
}

func TestConsumeUpdatesLevelsWithoutChangingOrder(t *testing.T) {
	book := testBook()
	order := restingOrder(1, constant.Sell, 100, 10)
	if err := book.Rest(order); err != nil {
		t.Fatal(err)
	}

	if err := book.Consume(1, 4); err != nil {
		t.Fatal(err)
	}
	if order.Remaining != 10 || order.State != constant.StateAccept {
		t.Fatalf("order changed by book = (%d,%v), want (10,accept)", order.Remaining, order.State)
	}
	assertLevels(t, book.Asks(1), []Level{{Price: 100, Amount: 6, OrderCount: 1}})

	order.Remaining = 6
	if err := book.Consume(1, 6); err != nil {
		t.Fatal(err)
	}
	if order.Remaining != 6 || order.State != constant.StateAccept {
		t.Fatalf("order changed by book = (%d,%v), want (6,accept)", order.Remaining, order.State)
	}
	if _, _, ok := book.BestAsk(); ok {
		t.Fatal("ask should be empty after full fill")
	}
}

func TestRemoveReturnsRemainingAndDeletesLevel(t *testing.T) {
	book := testBook()
	order := restingOrder(1, constant.Buy, 100, 10)
	if err := book.Rest(order); err != nil {
		t.Fatal(err)
	}
	order.Remaining = 7
	if err := book.Consume(1, 3); err != nil {
		t.Fatal(err)
	}

	remaining, err := book.Remove(1)
	if err != nil {
		t.Fatal(err)
	}
	if remaining != 7 {
		t.Fatalf("remaining = %d, want 7", remaining)
	}
	if order.State != constant.StateAccept {
		t.Fatalf("state = %v, want unchanged accept state", order.State)
	}
	if _, _, ok := book.BestBid(); ok {
		t.Fatal("bid should be empty")
	}
	if _, err := book.Remove(1); !errors.Is(err, constant.ErrOrderNotFound) {
		t.Fatalf("second Remove error = %v, want order not found", err)
	}
}

func TestChangeTrackingReturnsFinalAffectedLevels(t *testing.T) {
	book := testBook()
	book.BeginTracking()
	for _, order := range []*trading.Order{
		restingOrder(1, constant.Buy, 100, 2),
		restingOrder(2, constant.Buy, 100, 3),
		restingOrder(3, constant.Sell, 101, 4),
	} {
		if err := book.Rest(order); err != nil {
			t.Fatal(err)
		}
	}
	changes := book.EndTracking()

	assertLevels(t, changes.Bids, []Level{{Price: 100, Amount: 5, OrderCount: 2}})
	assertLevels(t, changes.Asks, []Level{{Price: 101, Amount: 4, OrderCount: 1}})

	book.BeginTracking()
	if err := book.Consume(1, 2); err != nil {
		t.Fatal(err)
	}
	changes = book.EndTracking()
	assertLevels(t, changes.Bids, []Level{{Price: 100, Amount: 3, OrderCount: 1}})

	book.BeginTracking()
	if _, err := book.Remove(2); err != nil {
		t.Fatal(err)
	}
	changes = book.EndTracking()
	assertLevels(t, changes.Bids, []Level{{Price: 100, Amount: 0, OrderCount: 0}})
}

func TestChangeSetMergeRetainsLatestLevel(t *testing.T) {
	changes := ChangeSet{
		Bids: []Level{{Price: 100, Amount: 2, OrderCount: 1}},
	}
	changes.Merge(ChangeSet{
		Bids: []Level{{Price: 100, Amount: 0, OrderCount: 0}},
		Asks: []Level{{Price: 101, Amount: 3, OrderCount: 1}},
	})

	assertLevels(t, changes.Bids, []Level{{Price: 100, Amount: 0, OrderCount: 0}})
	assertLevels(t, changes.Asks, []Level{{Price: 101, Amount: 3, OrderCount: 1}})
}

func TestAvailableRespectsLimitAndMarket(t *testing.T) {
	book := testBook()
	for _, order := range []*trading.Order{
		restingOrder(1, constant.Sell, 100, 2),
		restingOrder(2, constant.Sell, 105, 3),
		restingOrder(3, constant.Sell, 110, 100),
	} {
		if err := book.Rest(order); err != nil {
			t.Fatal(err)
		}
	}

	if got := book.Available(constant.Buy, 105, false); got != 5 {
		t.Fatalf("limited available = %d, want 5", got)
	}
	if got := book.Available(constant.Buy, 0, true); got != 105 {
		t.Fatalf("market available = %d, want 105", got)
	}
	if got := book.Available(constant.Buy, 0, false); got != 0 {
		t.Fatalf("zero-price limited available = %d, want 0", got)
	}
}

func TestRestRejectsInvalidOrders(t *testing.T) {
	book := testBook()
	order := restingOrder(1, constant.Buy, 100, 1)
	if err := book.Rest(order); err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name  string
		order *trading.Order
		want  error
	}{
		{name: "nil", order: nil, want: constant.ErrInvalidAmount},
		{name: "zero id", order: restingOrder(0, constant.Buy, 100, 1), want: constant.ErrInvalidOrderID},
		{name: "duplicate id", order: restingOrder(1, constant.Sell, 101, 1), want: constant.ErrDuplicateOrderID},
		{name: "zero remaining", order: func() *trading.Order {
			order := restingOrder(2, constant.Buy, 100, 1)
			order.Remaining = 0
			return order
		}(), want: constant.ErrInvalidAmount},
		{name: "market cannot rest", order: func() *trading.Order {
			order := restingOrder(2, constant.Buy, 0, 1)
			order.Type = constant.Market
			return order
		}(), want: constant.ErrMarketOrderCannotRest},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if err := book.Rest(test.order); !errors.Is(err, test.want) {
				t.Fatalf("error = %v, want %v", err, test.want)
			}
		})
	}
}

func assertLevels(t *testing.T, got, want []Level) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("levels = %+v, want %+v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("levels[%d] = %+v, want %+v", i, got[i], want[i])
		}
	}
}
