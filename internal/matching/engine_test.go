package matching

import (
	"testing"

	"my-go-exchange/internal/types"
)

func newTestEngine() *Engine {
	return NewEngine(&types.Instrument{Symbol: "BTC/USDT", Base: "BTC", Quote: "USDT"})
}

func tradesOf(events []Event) []*Trade {
	var out []*Trade
	for _, e := range events {
		if t, ok := e.(*Trade); ok {
			out = append(out, t)
		}
	}
	return out
}

func lastOfType[T Event](events []Event) T {
	var zero T
	for i := len(events) - 1; i >= 0; i-- {
		if v, ok := events[i].(T); ok {
			return v
		}
	}
	return zero
}

func TestLimitRestWhenNoCross(t *testing.T) {
	eng := newTestEngine()
	events := eng.Apply(&PlaceOrder{
		AccountID: 1,
		Side:      types.Buy,
		OrderType: types.Limit,
		Price:     100,
		Amount:    10,
		TIF:       types.GTC,
	})

	if len(events) != 1 {
		t.Fatalf("want 1 event, got %d", len(events))
	}
	if _, ok := events[0].(*OrderAccepted); !ok {
		t.Fatalf("want OrderAccepted, got %T", events[0])
	}
	if len(tradesOf(events)) != 0 {
		t.Fatalf("want no trades")
	}

	price, amt, ok := eng.ob.BestBid()
	if !ok || price != 100 || amt != 10 {
		t.Fatalf("BestBid = (%d,%d,%v), want (100,10,true)", price, amt, ok)
	}
}

func TestLimitPartialFillThenRest(t *testing.T) {
	eng := newTestEngine()
	eng.Apply(&PlaceOrder{
		AccountID: 1,
		Side:      types.Sell,
		OrderType: types.Limit,
		Price:     100,
		Amount:    4,
		TIF:       types.GTC,
	})

	events := eng.Apply(&PlaceOrder{
		AccountID: 2,
		Side:      types.Buy,
		OrderType: types.Limit,
		Price:     100,
		Amount:    10,
		TIF:       types.GTC,
	})

	trades := tradesOf(events)
	if len(trades) != 1 {
		t.Fatalf("want 1 trade, got %d", len(trades))
	}
	if trades[0].Price != 100 || trades[0].Amount != 4 {
		t.Fatalf("trade = (%d,%d), want (100,4)", trades[0].Price, trades[0].Amount)
	}
	if trades[0].TakerAccountID != 2 || trades[0].MakerAccountID != 1 {
		t.Fatalf("accounts = (%d,%d), want (2,1)", trades[0].TakerAccountID, trades[0].MakerAccountID)
	}

	// 剩余 6 应挂在买一
	price, amt, ok := eng.ob.BestBid()
	if !ok || price != 100 || amt != 6 {
		t.Fatalf("BestBid = (%d,%d,%v), want (100,6,true)", price, amt, ok)
	}
	if _, _, ok := eng.ob.BestAsk(); ok {
		t.Fatalf("asks should be empty")
	}
}

func TestMarketBuyMatchesAndDoesNotRest(t *testing.T) {
	eng := newTestEngine()
	eng.Apply(&PlaceOrder{
		AccountID: 1,
		Side:      types.Sell,
		OrderType: types.Limit,
		Price:     100,
		Amount:    10,
		TIF:       types.GTC,
	})
	eng.Apply(&PlaceOrder{
		AccountID: 1,
		Side:      types.Sell,
		OrderType: types.Limit,
		Price:     110,
		Amount:    10,
		TIF:       types.GTC,
	})

	events := eng.Apply(&PlaceOrder{
		AccountID: 2,
		Side:      types.Buy,
		OrderType: types.Market,
		Amount:    5,
		TIF:       types.GTC,
	})

	trades := tradesOf(events)
	if len(trades) != 1 {
		t.Fatalf("want 1 trade, got %d", len(trades))
	}
	if trades[0].Price != 100 || trades[0].Amount != 5 {
		t.Fatalf("trade = (%d,%d), want (100,5)", trades[0].Price, trades[0].Amount)
	}

	// 市价买 5 后剩余不入 book
	if _, _, ok := eng.ob.BestBid(); ok {
		t.Fatalf("market order must not rest on bid")
	}
	_, askAmt, ok := eng.ob.BestAsk()
	if !ok || askAmt != 5 {
		t.Fatalf("BestAsk amount = %d, want 5", askAmt)
	}
}

func TestMarketBuyInsufficientLiquidity(t *testing.T) {
	eng := newTestEngine()
	eng.Apply(&PlaceOrder{
		AccountID: 1,
		Side:      types.Sell,
		OrderType: types.Limit,
		Price:     100,
		Amount:    3,
		TIF:       types.GTC,
	})

	events := eng.Apply(&PlaceOrder{
		AccountID: 2,
		Side:      types.Buy,
		OrderType: types.Market,
		Amount:    10,
		TIF:       types.GTC,
	})

	trades := tradesOf(events)
	if len(trades) != 1 || trades[0].Amount != 3 {
		t.Fatalf("want 1 trade of 3, got %+v", trades)
	}
	// 剩余 7 作废，不能挂在 price=0
	if _, _, ok := eng.ob.BestBid(); ok {
		t.Fatalf("leftover market amount must not rest")
	}
	if cancel := lastOfType[*OrderCancelled](events); cancel == nil || cancel.Remaining != 7 {
		t.Fatalf("want OrderCancelled remaining=7, got %+v", cancel)
	}
}

func TestIOCDoesNotRest(t *testing.T) {
	eng := newTestEngine()
	eng.Apply(&PlaceOrder{
		AccountID: 1,
		Side:      types.Sell,
		OrderType: types.Limit,
		Price:     100,
		Amount:    2,
		TIF:       types.GTC,
	})

	events := eng.Apply(&PlaceOrder{
		AccountID: 2,
		Side:      types.Buy,
		OrderType: types.Limit,
		Price:     100,
		Amount:    10,
		TIF:       types.IOC,
	})

	if len(tradesOf(events)) != 1 {
		t.Fatalf("want 1 trade")
	}
	if cancel := lastOfType[*OrderCancelled](events); cancel == nil || cancel.Remaining != 8 {
		t.Fatalf("want cancel remaining=8, got %+v", cancel)
	}
	if _, _, ok := eng.ob.BestBid(); ok {
		t.Fatalf("IOC must not rest")
	}
}

func TestFOKRejectWhenCannotFill(t *testing.T) {
	eng := newTestEngine()
	eng.Apply(&PlaceOrder{
		AccountID: 1,
		Side:      types.Sell,
		OrderType: types.Limit,
		Price:     100,
		Amount:    5,
		TIF:       types.GTC,
	})

	events := eng.Apply(&PlaceOrder{
		AccountID: 2,
		Side:      types.Buy,
		OrderType: types.Limit,
		Price:     100,
		Amount:    10,
		TIF:       types.FOK,
	})

	if len(tradesOf(events)) != 0 {
		t.Fatalf("FOK reject must not produce trades")
	}
	rej := lastOfType[*OrderRejected](events)
	if rej == nil {
		t.Fatalf("want OrderRejected")
	}
	// maker 原样保留
	_, amt, ok := eng.ob.BestAsk()
	if !ok || amt != 5 {
		t.Fatalf("maker must remain, ask amount=%d ok=%v", amt, ok)
	}
}

func TestFOKFullFill(t *testing.T) {
	eng := newTestEngine()
	eng.Apply(&PlaceOrder{
		AccountID: 1,
		Side:      types.Sell,
		OrderType: types.Limit,
		Price:     100,
		Amount:    10,
		TIF:       types.GTC,
	})

	events := eng.Apply(&PlaceOrder{
		AccountID: 2,
		Side:      types.Buy,
		OrderType: types.Limit,
		Price:     100,
		Amount:    10,
		TIF:       types.FOK,
	})

	trades := tradesOf(events)
	if len(trades) != 1 || trades[0].Amount != 10 {
		t.Fatalf("want full fill trade, got %+v", trades)
	}
	if _, _, ok := eng.ob.BestAsk(); ok {
		t.Fatalf("ask should be gone")
	}
}

func TestCancel(t *testing.T) {
	eng := newTestEngine()
	eng.Apply(&PlaceOrder{
		AccountID: 1,
		Side:      types.Buy,
		OrderType: types.Limit,
		Price:     100,
		Amount:    10,
		TIF:       types.GTC,
	})

	events := eng.Apply(&CancelOrder{OrderID: 1})
	if _, ok := events[0].(*OrderCancelled); !ok {
		t.Fatalf("want OrderCancelled, got %T", events[0])
	}
	if _, _, ok := eng.ob.BestBid(); ok {
		t.Fatalf("bid should be empty after cancel")
	}
}

func TestTimePrioritySamePrice(t *testing.T) {
	eng := newTestEngine()
	eng.Apply(&PlaceOrder{
		AccountID: 1,
		Side:      types.Sell,
		OrderType: types.Limit,
		Price:     100,
		Amount:    3,
		TIF:       types.GTC,
	})
	eng.Apply(&PlaceOrder{
		AccountID: 2,
		Side:      types.Sell,
		OrderType: types.Limit,
		Price:     100,
		Amount:    3,
		TIF:       types.GTC,
	})

	events := eng.Apply(&PlaceOrder{
		AccountID: 3,
		Side:      types.Buy,
		OrderType: types.Limit,
		Price:     100,
		Amount:    3,
		TIF:       types.GTC,
	})

	trades := tradesOf(events)
	if len(trades) != 1 {
		t.Fatalf("want 1 trade, got %d", len(trades))
	}
	// 先挂的 account=1 应先成交
	if trades[0].MakerAccountID != 1 {
		t.Fatalf("maker account = %d, want 1 (time priority)", trades[0].MakerAccountID)
	}
}

func TestPricePriority(t *testing.T) {
	eng := newTestEngine()
	// 先挂高价卖，再挂低价卖 — 买盘应先吃低价
	eng.Apply(&PlaceOrder{
		AccountID: 1,
		Side:      types.Sell,
		OrderType: types.Limit,
		Price:     110,
		Amount:    5,
		TIF:       types.GTC,
	})
	eng.Apply(&PlaceOrder{
		AccountID: 2,
		Side:      types.Sell,
		OrderType: types.Limit,
		Price:     100,
		Amount:    5,
		TIF:       types.GTC,
	})

	events := eng.Apply(&PlaceOrder{
		AccountID: 3,
		Side:      types.Buy,
		OrderType: types.Limit,
		Price:     105,
		Amount:    5,
		TIF:       types.GTC,
	})

	trades := tradesOf(events)
	if len(trades) != 1 || trades[0].Price != 100 {
		t.Fatalf("want trade at 100, got %+v", trades)
	}
}

func TestRejectInvalidPlace(t *testing.T) {
	eng := newTestEngine()
	events := eng.Apply(&PlaceOrder{
		Side:      types.Buy,
		OrderType: types.Limit,
		Price:     0, // invalid
		Amount:    1,
		TIF:       types.GTC,
	})
	if _, ok := events[0].(*OrderRejected); !ok {
		t.Fatalf("want OrderRejected for zero limit price")
	}
}

func TestMultiLevelSweep(t *testing.T) {
	eng := newTestEngine()
	eng.Apply(&PlaceOrder{
		AccountID: 1,
		Side:      types.Sell,
		OrderType: types.Limit,
		Price:     100,
		Amount:    2,
		TIF:       types.GTC,
	})
	eng.Apply(&PlaceOrder{
		AccountID: 2,
		Side:      types.Sell,
		OrderType: types.Limit,
		Price:     101,
		Amount:    3,
		TIF:       types.GTC,
	})
	eng.Apply(&PlaceOrder{
		AccountID: 3,
		Side:      types.Sell,
		OrderType: types.Limit,
		Price:     102,
		Amount:    10,
		TIF:       types.GTC,
	})

	events := eng.Apply(&PlaceOrder{
		AccountID: 4,
		Side:      types.Buy,
		OrderType: types.Limit,
		Price:     101,
		Amount:    5,
		TIF:       types.GTC,
	})

	trades := tradesOf(events)
	if len(trades) != 2 {
		t.Fatalf("want 2 trades, got %d", len(trades))
	}
	if trades[0].Price != 100 || trades[0].Amount != 2 {
		t.Fatalf("trade0 = (%d,%d), want (100,2)", trades[0].Price, trades[0].Amount)
	}
	if trades[1].Price != 101 || trades[1].Amount != 3 {
		t.Fatalf("trade1 = (%d,%d), want (101,3)", trades[1].Price, trades[1].Amount)
	}
	price, _, ok := eng.ob.BestAsk()
	if !ok || price != 102 {
		t.Fatalf("BestAsk = (%d,%v), want 102", price, ok)
	}
}
