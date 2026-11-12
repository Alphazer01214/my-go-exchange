package market_test

import (
	"context"
	"testing"
	"time"

	"my-go-exchange/internal/constant"
	"my-go-exchange/internal/market"
	"my-go-exchange/internal/marketdata"
	"my-go-exchange/internal/trading"
)

func TestLiquidationScannerClosesLongAndShort(t *testing.T) {
	exchange, local := newLiquidationService(t)
	ctx := context.Background()

	openPosition(t, local, 1, "open-short", constant.Sell, constant.Short)
	openPosition(t, local, 2, "open-long", constant.Buy, constant.Long)

	if err := local.ApplyBook(ctx, liquidationBook(89, 90, 2)); err != nil {
		t.Fatal(err)
	}
	if err := exchange.ScanLiquidations(ctx); err != nil {
		t.Fatal(err)
	}

	assertNoPosition(t, exchange, 2, constant.Long)
	assertPositionExists(t, exchange, 1, constant.Short)
	assertAvailable(t, exchange, 2, 978)

	// A second scan must be a no-op after the long position is closed.
	if err := exchange.ScanLiquidations(ctx); err != nil {
		t.Fatal(err)
	}
	assertAvailable(t, exchange, 2, 978)

	if err := local.ApplyBook(ctx, liquidationBook(111, 112, 2)); err != nil {
		t.Fatal(err)
	}
	if err := exchange.ScanLiquidations(ctx); err != nil {
		t.Fatal(err)
	}

	assertNoPosition(t, exchange, 1, constant.Short)
	assertAvailable(t, exchange, 1, 976)
}

func TestLiquidationScannerSkipsPositionsThatAreNotTriggered(t *testing.T) {
	exchange, local := newLiquidationService(t)
	ctx := context.Background()

	openPosition(t, local, 2, "open-short", constant.Sell, constant.Short)
	openPosition(t, local, 1, "open-long", constant.Buy, constant.Long)
	if err := local.ApplyBook(ctx, liquidationBook(91, 92, 2)); err != nil {
		t.Fatal(err)
	}
	if err := exchange.ScanLiquidations(ctx); err != nil {
		t.Fatal(err)
	}

	assertPositionExists(t, exchange, 1, constant.Long)
	assertAvailable(t, exchange, 1, 980)
}

func TestMarketStartRunsLiquidationScannerEvery500Milliseconds(t *testing.T) {
	if market.LiquidationScanInterval != 500*time.Millisecond {
		t.Fatalf("liquidation scan interval = %s, want 500ms", market.LiquidationScanInterval)
	}

	exchange, local := newLiquidationService(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	openPosition(t, local, 2, "open-short", constant.Sell, constant.Short)
	openPosition(t, local, 1, "open-long", constant.Buy, constant.Long)
	if err := exchange.RegisterSource("BTC/USDT", quietMarketSource{}); err != nil {
		t.Fatal(err)
	}
	if err := exchange.Start(ctx); err != nil {
		t.Fatal(err)
	}
	if err := local.ApplyBook(ctx, liquidationBook(89, 90, 2)); err != nil {
		t.Fatal(err)
	}

	deadline := time.Now().Add(2 * time.Second)
	for positionExists(exchange, 1, constant.Long) {
		if time.Now().After(deadline) {
			t.Fatal("position was not liquidated by the automatic scanner")
		}
		time.Sleep(25 * time.Millisecond)
	}
	assertAvailable(t, exchange, 1, 978)
}

type quietMarketSource struct{}

func (quietMarketSource) Run(ctx context.Context, _ chan<- market.L2Book) error {
	<-ctx.Done()
	return nil
}

func newLiquidationService(t *testing.T) (*market.Market, *market.LocalSymbolMarket) {
	t.Helper()
	exchange := market.NewMarket()
	local, err := exchange.RegisterLocal(&trading.Instrument{
		Symbol: "BTC/USDT",
		Base:   "BTC",
		Quote:  "USDT",
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, accountID := range []int64{1, 2, 3} {
		if err := exchange.Deposit(context.Background(), accountID, "USDT", 1000); err != nil {
			t.Fatal(err)
		}
	}
	return exchange, local
}

func openPosition(
	t *testing.T,
	local *market.LocalSymbolMarket,
	accountID int64,
	clientOrderID string,
	orderSide constant.OrderSide,
	positionSide constant.PositionSide,
) {
	t.Helper()
	if _, err := local.Place(context.Background(), market.PlaceReq{
		AccountID:     accountID,
		ClientOrderID: clientOrderID,
		Type:          constant.Limit,
		Symbol:        "BTC/USDT",
		OrderSide:     orderSide,
		PositionSide:  positionSide,
		Price:         100,
		Amount:        2,
		Leverage:      10,
	}); err != nil {
		t.Fatal(err)
	}
}

func liquidationBook(bid, ask, amount int64) marketdata.Book {
	return marketdata.Book{
		Symbol:    "BTC/USDT",
		Bids:      []marketdata.Level{{Price: bid, Amount: amount}},
		Asks:      []marketdata.Level{{Price: ask, Amount: amount}},
		Timestamp: time.Now().UnixMilli(),
	}
}

func assertNoPosition(
	t *testing.T,
	exchange *market.Market,
	accountID int64,
	positionSide constant.PositionSide,
) {
	t.Helper()
	if positionExists(exchange, accountID, positionSide) {
		t.Fatalf("account %d %s position still exists", accountID, positionSide)
	}
}

func assertPositionExists(
	t *testing.T,
	exchange *market.Market,
	accountID int64,
	positionSide constant.PositionSide,
) {
	t.Helper()
	if !positionExists(exchange, accountID, positionSide) {
		t.Fatalf("account %d %s position not found", accountID, positionSide)
	}
}

func positionExists(
	exchange *market.Market,
	accountID int64,
	positionSide constant.PositionSide,
) bool {
	_, ok := exchange.Position(context.Background(), accountID, "BTC/USDT", positionSide)
	return ok
}

func assertAvailable(t *testing.T, exchange *market.Market, accountID, want int64) {
	t.Helper()
	balance := exchange.Balance(context.Background(), accountID, "USDT")
	if balance.Available != want || balance.Frozen != 0 {
		t.Fatalf(
			"account %d balance = available %d frozen %d, want available %d frozen 0",
			accountID,
			balance.Available,
			balance.Frozen,
			want,
		)
	}
}
