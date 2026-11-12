package market_test

import (
	"context"
	"testing"

	"my-go-exchange/internal/constant"
	"my-go-exchange/internal/market"
	"my-go-exchange/internal/marketdata"
	"my-go-exchange/internal/trading"
)

func TestMarketSharesAccountAndPortfolioAcrossSymbolMarkets(t *testing.T) {
	exchange := market.NewMarket()
	for _, inst := range []trading.Instrument{
		{Symbol: "BTC/USDT", Base: "BTC", Quote: "USDT"},
		{Symbol: "ETH/USDT", Base: "ETH", Quote: "USDT"},
	} {
		if _, err := exchange.RegisterLocal(&inst); err != nil {
			t.Fatalf("RegisterLocal(%s): %v", inst.Symbol, err)
		}
	}

	for _, accountID := range []int64{1, 2} {
		if err := exchange.Deposit(context.Background(), accountID, "USDT", 1000); err != nil {
			t.Fatal(err)
		}
	}

	if _, err := exchange.Place(context.Background(), market.PlaceReq{
		AccountID:     1,
		ClientOrderID: "btc-open",
		Type:          constant.Limit,
		Symbol:        "BTC/USDT",
		OrderSide:     constant.Buy,
		PositionSide:  constant.Long,
		Price:         100,
		Amount:        2,
		Leverage:      1,
	}); err != nil {
		t.Fatal(err)
	}

	if _, err := exchange.Place(context.Background(), market.PlaceReq{
		AccountID:     2,
		ClientOrderID: "btc-take",
		Type:          constant.Limit,
		Symbol:        "BTC/USDT",
		OrderSide:     constant.Sell,
		PositionSide:  constant.Short,
		Price:         100,
		Amount:        2,
		Leverage:      1,
	}); err != nil {
		t.Fatal(err)
	}

	if _, err := exchange.Place(context.Background(), market.PlaceReq{
		AccountID:     1,
		ClientOrderID: "eth-open",
		Type:          constant.Limit,
		Symbol:        "ETH/USDT",
		OrderSide:     constant.Buy,
		PositionSide:  constant.Long,
		Price:         50,
		Amount:        2,
		Leverage:      1,
	}); err != nil {
		t.Fatal(err)
	}

	if _, err := exchange.Place(context.Background(), market.PlaceReq{
		AccountID:     2,
		ClientOrderID: "eth-take",
		Type:          constant.Limit,
		Symbol:        "ETH/USDT",
		OrderSide:     constant.Sell,
		PositionSide:  constant.Short,
		Price:         50,
		Amount:        2,
		Leverage:      1,
	}); err != nil {
		t.Fatal(err)
	}

	balance := exchange.Balance(context.Background(), 1, "USDT")
	if balance.Available != 700 || balance.Frozen != 0 {
		t.Fatalf("balance = %+v, want available 700 and frozen 0", balance)
	}

	for _, symbol := range []string{"BTC/USDT", "ETH/USDT"} {
		if _, ok := exchange.Position(context.Background(), 1, symbol, constant.Long); !ok {
			t.Fatalf("position %s not found", symbol)
		}
	}
}

func TestMarketUsesGloballyUniqueOrderIDsAcrossSymbols(t *testing.T) {
	exchange := market.NewMarket()
	for _, inst := range []trading.Instrument{
		{Symbol: "BTC/USDT", Base: "BTC", Quote: "USDT"},
		{Symbol: "ETH/USDT", Base: "ETH", Quote: "USDT"},
	} {
		if _, err := exchange.RegisterLocal(&inst); err != nil {
			t.Fatal(err)
		}
	}
	if err := exchange.Deposit(context.Background(), 1, "USDT", 1000); err != nil {
		t.Fatal(err)
	}

	btcOrder, err := exchange.Place(context.Background(), market.PlaceReq{
		AccountID:     1,
		ClientOrderID: "btc-resting",
		Type:          constant.Limit,
		Symbol:        "BTC/USDT",
		OrderSide:     constant.Buy,
		PositionSide:  constant.Long,
		Price:         90,
		Amount:        1,
		Leverage:      1,
	})
	if err != nil {
		t.Fatal(err)
	}
	ethOrder, err := exchange.Place(context.Background(), market.PlaceReq{
		AccountID:     1,
		ClientOrderID: "eth-resting",
		Type:          constant.Limit,
		Symbol:        "ETH/USDT",
		OrderSide:     constant.Buy,
		PositionSide:  constant.Long,
		Price:         40,
		Amount:        1,
		Leverage:      1,
	})
	if err != nil {
		t.Fatal(err)
	}

	if btcOrder.ID == ethOrder.ID {
		t.Fatalf("order IDs collide across symbols: %d", btcOrder.ID)
	}
	if canceled, err := exchange.Cancel(context.Background(), 1, btcOrder.ID); err != nil {
		t.Fatal(err)
	} else if canceled.Symbol != "BTC/USDT" || canceled.State != constant.StateCanceled {
		t.Fatalf("canceled BTC order = %+v", canceled)
	}
	if canceled, err := exchange.Cancel(context.Background(), 1, ethOrder.ID); err != nil {
		t.Fatal(err)
	} else if canceled.Symbol != "ETH/USDT" || canceled.State != constant.StateCanceled {
		t.Fatalf("canceled ETH order = %+v", canceled)
	}
}

func TestLocalSymbolMarketCanTradeAgainstMirrorLiquidity(t *testing.T) {
	exchange := market.NewMarket()
	local, err := exchange.RegisterLocal(&trading.Instrument{
		Symbol: "BTC/USDT",
		Base:   "BTC",
		Quote:  "USDT",
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := exchange.Deposit(context.Background(), 1, "USDT", 1000); err != nil {
		t.Fatal(err)
	}

	err = local.ApplyBook(context.Background(), marketdata.Book{
		Symbol: "BTC/USDT",
		Bids:   []marketdata.Level{{Price: 100, Amount: 1}},
		Asks:   []marketdata.Level{{Price: 101, Amount: 1}},
	})
	if err != nil {
		t.Fatal(err)
	}

	order, err := exchange.Place(context.Background(), market.PlaceReq{
		AccountID:     1,
		ClientOrderID: "take-mirror",
		Type:          constant.Limit,
		Symbol:        "BTC/USDT",
		OrderSide:     constant.Buy,
		PositionSide:  constant.Long,
		Price:         101,
		Amount:        1,
		Leverage:      1,
	})
	if err != nil {
		t.Fatal(err)
	}
	if order.State != constant.StateFilled {
		t.Fatalf("order state = %s, want filled", order.State)
	}
	if _, ok := exchange.Position(context.Background(), 1, "BTC/USDT", constant.Long); !ok {
		t.Fatal("position against mirror liquidity not found")
	}
}
