package market_test

import (
	"context"
	"errors"
	"my-go-exchange/internal/trading"
	"testing"

	"my-go-exchange/internal/constant"
	"my-go-exchange/internal/entity"
	"my-go-exchange/internal/market"
)

type testMarket struct {
	*market.Market
}

func (m *testMarket) Deposit(ctx context.Context, accountID int64, amount int64) error {
	return m.Market.Deposit(ctx, accountID, "USDT", amount)
}

func (m *testMarket) Balance(ctx context.Context, accountID int64) entity.Balance {
	return m.Market.Balance(ctx, accountID, "USDT")
}

func newService() *testMarket {
	instrument := trading.Instrument{
		Symbol: "BTC/USDT",
		Base:   "BTC",
		Quote:  "USDT",
	}
	exchange := market.NewMarket()
	if _, err := exchange.RegisterLocal(&instrument); err != nil {
		panic(err)
	}
	for _, accountID := range []int64{1, 2, 3} {
		if err := exchange.Deposit(context.Background(), accountID, "USDT", 1000); err != nil {
			panic(err)
		}
	}
	return &testMarket{Market: exchange}
}

func place(
	service *testMarket,
	accountID int64,
	clientOrderID string,
	side constant.OrderSide,
	price, amount int64,
) (trading.Order, error) {
	return service.Place(context.Background(), market.PlaceReq{
		AccountID:     accountID,
		ClientOrderID: clientOrderID,
		Type:          constant.Limit,
		Symbol:        "BTC/USDT",
		OrderSide:     side,
		PositionSide:  positionSideFor(side),
		Leverage:      1,
		Price:         price,
		Amount:        amount,
	})
}

func positionSideFor(side constant.OrderSide) constant.PositionSide {
	if side == constant.Buy {
		return constant.Long
	}
	return constant.Short
}

func TestPlaceMatchAndCancel(t *testing.T) {
	service := newService()

	sell, err := place(service, 1, "sell-1", constant.Sell, 100, 4)
	if err != nil {
		t.Fatal(err)
	}
	if sell.State != constant.StateInBook || sell.Remaining != 4 {
		t.Fatalf("sell order = %+v", sell)
	}

	buy, err := place(service, 2, "buy-1", constant.Buy, 100, 10)
	if err != nil {
		t.Fatal(err)
	}
	if buy.State != constant.StatePartial || buy.Remaining != 6 {
		t.Fatalf("buy order = %+v", buy)
	}

	sell, err = service.GetOrder(context.Background(), 1, sell.ID)
	if err != nil {
		t.Fatal(err)
	}
	if sell.State != constant.StateFilled || sell.Remaining != 0 {
		t.Fatalf("sell order after match = %+v", sell)
	}

	canceled, err := service.Cancel(context.Background(), 2, buy.ID)
	if err != nil {
		t.Fatal(err)
	}
	if canceled.State != constant.StateCanceled || canceled.Remaining != 6 {
		t.Fatalf("canceled order = %+v", canceled)
	}
}

func TestDuplicateClientOrderIDReturnsExistingOrder(t *testing.T) {
	service := newService()

	first, err := place(service, 1, "client-1", constant.Buy, 100, 10)
	if err != nil {
		t.Fatal(err)
	}

	second, err := place(service, 1, "client-1", constant.Buy, 99, 5)
	if err != nil {
		t.Fatal(err)
	}
	if second.ID != first.ID || second.Price != first.Price || second.Amount != first.Amount {
		t.Fatalf("duplicate request returned %+v, want %+v", second, first)
	}
	if first.ClientOrderID != "client-1" {
		t.Fatalf("client order ID = %q, want client-1", first.ClientOrderID)
	}
}

func TestCancelRejectsAnotherAccount(t *testing.T) {
	service := newService()

	order, err := place(service, 1, "client-1", constant.Buy, 100, 10)
	if err != nil {
		t.Fatal(err)
	}

	_, err = service.Cancel(context.Background(), 2, order.ID)
	if !errors.Is(err, constant.ErrOrderForbidden) {
		t.Fatalf("Cancel error = %v, want order forbidden", err)
	}

	current, err := service.GetOrder(context.Background(), 1, order.ID)
	if err != nil {
		t.Fatal(err)
	}
	if current.State != constant.StateInBook {
		t.Fatalf("order state = %s, want inbook", current.State)
	}
}

func TestRejectedOrderIsNotStored(t *testing.T) {
	service := newService()

	_, err := place(service, 1, "client-1", constant.Buy, 100, 0)
	if !errors.Is(err, constant.ErrOrderRejected) {
		t.Fatalf("invalid amount error = %v, want order rejected", err)
	}

	order, err := place(service, 1, "client-1", constant.Buy, 100, 1)
	if err != nil {
		t.Fatal(err)
	}
	if order.State != constant.StateInBook {
		t.Fatalf("order state = %s, want inbook", order.State)
	}
}

func TestTradeSettlesOpenPositionAndPriceImprovement(t *testing.T) {
	service := newService()

	if _, err := place(service, 1, "sell-short", constant.Sell, 100, 2); err != nil {
		t.Fatal(err)
	}
	if _, err := place(service, 2, "buy-long", constant.Buy, 110, 2); err != nil {
		t.Fatal(err)
	}

	assertBalance(t, service, 1, 800, 0)
	assertBalance(t, service, 2, 800, 0)
	assertPosition(t, service, 1, constant.Short, 2, 200)
	assertPosition(t, service, 2, constant.Long, 2, 200)
}

func TestOpenShortTakesMarginFromAvailableWhenTradePriceExceedsLimit(t *testing.T) {
	service := newService()

	if _, err := place(service, 1, "buy-long", constant.Buy, 100, 2); err != nil {
		t.Fatal(err)
	}
	if _, err := place(service, 2, "sell-short", constant.Sell, 90, 2); err != nil {
		t.Fatal(err)
	}

	assertBalance(t, service, 1, 800, 0)
	assertBalance(t, service, 2, 800, 0)
	assertPosition(t, service, 1, constant.Long, 2, 200)
	assertPosition(t, service, 2, constant.Short, 2, 200)
}

func TestTradeSettlesClosedMarginAndRealizedPnL(t *testing.T) {
	service := newService()

	if _, err := place(service, 1, "open-short", constant.Sell, 100, 4); err != nil {
		t.Fatal(err)
	}
	if _, err := place(service, 2, "open-long", constant.Buy, 100, 4); err != nil {
		t.Fatal(err)
	}

	_, err := service.Place(context.Background(), market.PlaceReq{
		AccountID:     2,
		ClientOrderID: "close-long",
		Type:          constant.Limit,
		Symbol:        "BTC/USDT",
		OrderSide:     constant.Sell,
		PositionSide:  constant.Long,
		Leverage:      1,
		Price:         110,
		Amount:        1,
	})
	if err != nil {
		t.Fatal(err)
	}

	_, err = service.Place(context.Background(), market.PlaceReq{
		AccountID:     3,
		ClientOrderID: "take-close",
		Type:          constant.Limit,
		Symbol:        "BTC/USDT",
		OrderSide:     constant.Buy,
		PositionSide:  constant.Long,
		Leverage:      1,
		Price:         110,
		Amount:        1,
	})
	if err != nil {
		t.Fatal(err)
	}

	assertBalance(t, service, 2, 710, 0)
	assertBalance(t, service, 3, 890, 0)
	assertPosition(t, service, 2, constant.Long, 3, 300)
	assertPosition(t, service, 3, constant.Long, 1, 110)
}

func TestTradeSettlesEveryMakerFill(t *testing.T) {
	service := newService()

	if _, err := place(service, 1, "maker-1", constant.Sell, 100, 2); err != nil {
		t.Fatal(err)
	}
	if _, err := place(service, 2, "maker-2", constant.Sell, 101, 3); err != nil {
		t.Fatal(err)
	}
	if _, err := place(service, 3, "taker", constant.Buy, 101, 5); err != nil {
		t.Fatal(err)
	}

	assertBalance(t, service, 1, 800, 0)
	assertBalance(t, service, 2, 697, 0)
	assertBalance(t, service, 3, 497, 0)
	assertPosition(t, service, 1, constant.Short, 2, 200)
	assertPosition(t, service, 2, constant.Short, 3, 303)
	assertPosition(t, service, 3, constant.Long, 5, 503)
}

func assertBalance(
	t *testing.T,
	service *testMarket,
	accountID int64,
	available int64,
	frozen int64,
) {
	t.Helper()
	balance := service.Balance(context.Background(), accountID)
	if balance.Available != available || balance.Frozen != frozen {
		t.Fatalf(
			"account %d balance = %+v, want available %d and frozen %d",
			accountID,
			balance,
			available,
			frozen,
		)
	}
}

func assertPosition(
	t *testing.T,
	service *testMarket,
	accountID int64,
	side constant.PositionSide,
	amount int64,
	margin int64,
) {
	t.Helper()
	position, ok := service.Position(
		context.Background(),
		accountID,
		"BTC/USDT",
		side,
	)
	if !ok {
		t.Fatalf("account %d %s position not found", accountID, side)
	}
	if position.Amount != amount || position.Margin != margin {
		t.Fatalf(
			"account %d %s position = %+v, want amount %d and margin %d",
			accountID,
			side,
			position,
			amount,
			margin,
		)
	}
}
