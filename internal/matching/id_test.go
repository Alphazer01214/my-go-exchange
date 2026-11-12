package matching

import (
	"testing"

	"my-go-exchange/internal/constant"
	"my-go-exchange/internal/trading"
)

func TestIDAllocatorIsGlobalAcrossEngines(t *testing.T) {
	ids := NewIDAllocator()
	btc := NewEngine(&trading.Instrument{Symbol: "BTC/USDT"}, ids)
	eth := NewEngine(&trading.Instrument{Symbol: "ETH/USDT"}, ids)

	btcSell := acceptedOrder(t, applyLimit(btc, constant.Sell, 100, 1))
	ethSell := acceptedOrder(t, applyLimit(eth, constant.Sell, 100, 1))
	if btcSell.ID == ethSell.ID {
		t.Fatalf("order IDs collide across engines: %d", btcSell.ID)
	}

	btcBuyEvents := applyLimit(btc, constant.Buy, 100, 1)
	ethBuyEvents := applyLimit(eth, constant.Buy, 100, 1)
	btcBuy := acceptedOrder(t, btcBuyEvents)
	ethBuy := acceptedOrder(t, ethBuyEvents)
	if btcBuy.ID == ethBuy.ID || btcBuy.ID == btcSell.ID || btcBuy.ID == ethSell.ID ||
		ethBuy.ID == btcSell.ID || ethBuy.ID == ethSell.ID {
		t.Fatalf("order IDs are not globally unique: %d %d %d %d",
			btcSell.ID, ethSell.ID, btcBuy.ID, ethBuy.ID)
	}

	btcTrade := onlyTrade(t, btcBuyEvents)
	ethTrade := onlyTrade(t, ethBuyEvents)
	if btcTrade.TradeID == ethTrade.TradeID {
		t.Fatalf("trade IDs collide across engines: %d", btcTrade.TradeID)
	}
}

func applyLimit(engine *Engine, side constant.OrderSide, price, amount int64) []Event {
	positionSide := constant.Long
	if side == constant.Sell {
		positionSide = constant.Short
	}
	return engine.Apply(&PlaceOrder{
		AccountID:     1,
		ClientOrderID: "test",
		OrderSide:     side,
		PositionSide:  positionSide,
		Type:          constant.Limit,
		Price:         price,
		Amount:        amount,
		Leverage:      1,
	})
}

func acceptedOrder(t *testing.T, events []Event) *trading.Order {
	t.Helper()
	for _, event := range events {
		if accepted, ok := event.(*OrderAccepted); ok {
			return &trading.Order{ID: accepted.OrderID}
		}
	}
	t.Fatalf("events do not contain an accepted order: %+v", events)
	return nil
}

func onlyTrade(t *testing.T, events []Event) *TradeEvent {
	t.Helper()
	for _, event := range events {
		if trade, ok := event.(*TradeEvent); ok {
			return trade
		}
	}
	t.Fatalf("events do not contain a trade: %+v", events)
	return nil
}
