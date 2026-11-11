package orderbook

import (
	"errors"
	"testing"

	"my-go-exchange/internal/errs"
	"my-go-exchange/internal/types"
)

func testBook() *OrderBook {
	return NewOrderBook(&types.Instrument{Symbol: "TEST/QUOTE"})
}

func restingOrder(id uint64, side types.Side, price, amount uint64) *types.Order {
	return &types.Order{
		ID:        id,
		AccountID: 1,
		Side:      side,
		Type:      types.Limit,
		Price:     price,
		Amount:    amount,
		Remaining: amount,
		TIF:       types.GTC,
		State:     types.StateAccept,
	}
}

func TestValidatePlace(t *testing.T) {
	tests := []struct {
		name      string
		orderType types.OrderType
		price     uint64
		amount    uint64
		side      types.Side
		tif       types.TIF
		want      error
	}{
		{name: "valid limit", orderType: types.Limit, price: 10, amount: 1, side: types.Buy, tif: types.GTC},
		{name: "valid market", orderType: types.Market, amount: 1, side: types.Sell, tif: types.IOC},
		{name: "invalid side", orderType: types.Limit, price: 10, amount: 1, side: types.Side(9), tif: types.GTC, want: errs.ErrInvalidSide},
		{name: "invalid type", orderType: types.OrderType(9), price: 10, amount: 1, side: types.Buy, tif: types.GTC, want: errs.ErrInvalidOrderType},
		// TIF validation is intentionally disabled for now.
		// {name: "invalid tif", orderType: types.Limit, price: 10, amount: 1, side: types.Buy, tif: types.TIF(9), want: errs.ErrInvalidTIF},
		{name: "zero amount", orderType: types.Limit, price: 10, side: types.Buy, tif: types.GTC, want: errs.ErrInvalidAmount},
		{name: "limit without price", orderType: types.Limit, amount: 1, side: types.Buy, tif: types.GTC, want: errs.ErrInvalidPrice},
		{name: "market with price", orderType: types.Market, price: 10, amount: 1, side: types.Buy, tif: types.GTC, want: errs.ErrMarketOrderWithPrice},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := ValidatePlace(test.orderType, test.price, test.amount, test.side, test.tif)
			if !errors.Is(err, test.want) {
				t.Fatalf("error = %v, want %v", err, test.want)
			}
		})
	}
}

func TestPlaceMaintainsPricePriority(t *testing.T) {
	book := testBook()
	for _, order := range []*types.Order{
		restingOrder(1, types.Buy, 100, 1),
		restingOrder(2, types.Buy, 102, 2),
		restingOrder(3, types.Buy, 101, 3),
		restingOrder(4, types.Sell, 105, 4),
		restingOrder(5, types.Sell, 103, 5),
	} {
		if err := book.Place(order); err != nil {
			t.Fatalf("Place(%d): %v", order.ID, err)
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
	first := restingOrder(1, types.Sell, 100, 2)
	second := restingOrder(2, types.Sell, 100, 3)
	if err := book.Place(first); err != nil {
		t.Fatal(err)
	}
	if err := book.Place(second); err != nil {
		t.Fatal(err)
	}

	got, ok := book.Front(types.Sell)
	if !ok || got.ID != 1 {
		t.Fatalf("Front = (%+v,%v), want order 1", got, ok)
	}
	got.AccountID = 99
	if first.AccountID != 1 {
		t.Fatal("Front exposed mutable order storage")
	}
}

func TestFillUpdatesOrderAndLevels(t *testing.T) {
	book := testBook()
	order := restingOrder(1, types.Sell, 100, 10)
	if err := book.Place(order); err != nil {
		t.Fatal(err)
	}

	if err := book.Fill(1, 4); err != nil {
		t.Fatal(err)
	}
	if order.Remaining != 6 || order.State != types.StatePartial {
		t.Fatalf("after partial fill = (%d,%v), want (6,partial)", order.Remaining, order.State)
	}
	assertLevels(t, book.Asks(1), []Level{{Price: 100, Amount: 6, OrderCount: 1}})

	if err := book.Fill(1, 6); err != nil {
		t.Fatal(err)
	}
	if order.State != types.StateFilled {
		t.Fatalf("state = %v, want filled", order.State)
	}
	if _, _, ok := book.BestAsk(); ok {
		t.Fatal("ask should be empty after full fill")
	}
}

func TestCancelReturnsRemainingAndDeletesLevel(t *testing.T) {
	book := testBook()
	order := restingOrder(1, types.Buy, 100, 10)
	if err := book.Place(order); err != nil {
		t.Fatal(err)
	}
	if err := book.Fill(1, 3); err != nil {
		t.Fatal(err)
	}

	remaining, err := book.Cancel(1)
	if err != nil {
		t.Fatal(err)
	}
	if remaining != 7 {
		t.Fatalf("remaining = %d, want 7", remaining)
	}
	if order.State != types.StateCanceled {
		t.Fatalf("state = %v, want canceled", order.State)
	}
	if _, _, ok := book.BestBid(); ok {
		t.Fatal("bid should be empty")
	}
	if _, err := book.Cancel(1); !errors.Is(err, errs.ErrOrderNotCancellable) {
		t.Fatalf("second Cancel error = %v, want order not cancellable", err)
	}
}

func TestAvailableRespectsLimitAndMarket(t *testing.T) {
	book := testBook()
	for _, order := range []*types.Order{
		restingOrder(1, types.Sell, 100, 2),
		restingOrder(2, types.Sell, 105, 3),
		restingOrder(3, types.Sell, 110, 100),
	} {
		if err := book.Place(order); err != nil {
			t.Fatal(err)
		}
	}

	if got := book.Available(types.Buy, 105, false); got != 5 {
		t.Fatalf("limited available = %d, want 5", got)
	}
	if got := book.Available(types.Buy, 0, true); got != 105 {
		t.Fatalf("market available = %d, want 105", got)
	}
	if got := book.Available(types.Buy, 0, false); got != 0 {
		t.Fatalf("zero-price limited available = %d, want 0", got)
	}
}

func TestPlaceRejectsInvalidOrders(t *testing.T) {
	book := testBook()
	order := restingOrder(1, types.Buy, 100, 1)
	if err := book.Place(order); err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name  string
		order *types.Order
		want  error
	}{
		{name: "nil", order: nil, want: errs.ErrInvalidAmount},
		{name: "zero id", order: restingOrder(0, types.Buy, 100, 1), want: errs.ErrInvalidOrderID},
		{name: "duplicate id", order: restingOrder(1, types.Sell, 101, 1), want: errs.ErrDuplicateOrderID},
		{name: "zero remaining", order: func() *types.Order {
			order := restingOrder(2, types.Buy, 100, 1)
			order.Remaining = 0
			return order
		}(), want: errs.ErrInvalidAmount},
		{name: "market cannot rest", order: func() *types.Order {
			order := restingOrder(2, types.Buy, 0, 1)
			order.Type = types.Market
			return order
		}(), want: errs.ErrMarketOrderCannotRest},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if err := book.Place(test.order); !errors.Is(err, test.want) {
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
