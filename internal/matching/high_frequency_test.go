package matching

import (
	"fmt"
	"testing"

	"my-go-exchange/internal/constant"
	"my-go-exchange/internal/orderbook"
)

func TestHighFrequencyRestingOrdersAndCancels(t *testing.T) {
	const orderCount = 12_000
	engine := matchingEngine()
	expectedBids := make(map[int64]orderbook.Level)
	expectedAsks := make(map[int64]orderbook.Level)
	orderIDs := make([]int64, 0, orderCount)
	orderAmounts := make(map[int64]int64, orderCount)
	previousSeq := int64(0)

	for i := 0; i < orderCount; i++ {
		side := constant.Sell
		price := int64(110 + i%10)
		if i%2 == 0 {
			side = constant.Buy
			price = int64(90 + i%10)
		}
		amount := int64(1 + i%7)

		events := engine.Apply(&PlaceOrder{
			AccountID:     int64(1 + i%1000),
			ClientOrderID: fmt.Sprintf("hf-%d", i),
			OrderSide:     side,
			PositionSide:  positionSideForTest(side),
			Type:          constant.Limit,
			Price:         price,
			Amount:        amount,
			Leverage:      1,
		})
		if len(events) != 1 {
			t.Fatalf("place %d events = %+v, want accepted only", i, events)
		}
		accepted, ok := events[0].(*OrderAccepted)
		if !ok {
			t.Fatalf("place %d event = %T, want OrderAccepted", i, events[0])
		}
		if accepted.Seq <= previousSeq {
			t.Fatalf("place %d sequence = %d, previous = %d", i, accepted.Seq, previousSeq)
		}
		previousSeq = accepted.Seq

		order := engine.GetOrder(accepted.OrderID)
		if order == nil || order.State != constant.StateInBook || order.Remaining != amount {
			t.Fatalf("place %d order = %+v", i, order)
		}
		orderIDs = append(orderIDs, accepted.OrderID)
		orderAmounts[accepted.OrderID] = amount

		levels := expectedBids
		if side == constant.Sell {
			levels = expectedAsks
		}
		level := levels[price]
		level.Price = price
		level.Amount += amount
		level.OrderCount++
		levels[price] = level

		if (i+1)%2_000 == 0 {
			assertBookAggregates(t, engine, expectedBids, expectedAsks)
		}
	}
	assertBookAggregates(t, engine, expectedBids, expectedAsks)

	for i, orderID := range orderIDs {
		if i%3 != 0 {
			continue
		}
		order := engine.GetOrder(orderID)
		if order == nil {
			t.Fatalf("order %d missing before cancel", orderID)
		}
		price, side, amount := order.Price, order.OrderSide, order.Remaining

		events := engine.Apply(&CancelOrder{OrderID: orderID})
		if len(events) != 1 {
			t.Fatalf("cancel %d events = %+v", orderID, events)
		}
		canceled, ok := events[0].(*OrderCanceled)
		if !ok || canceled.OrderID != orderID || canceled.Remaining != amount ||
			canceled.Reason != "user_canceled" {
			t.Fatalf("cancel %d event = %+v", orderID, events[0])
		}
		if order.State != constant.StateCanceled || order.Remaining != amount {
			t.Fatalf("cancel %d order = %+v", orderID, order)
		}

		levels := expectedBids
		if side == constant.Sell {
			levels = expectedAsks
		}
		level := levels[price]
		level.Amount -= amount
		level.OrderCount--
		if level.OrderCount == 0 {
			delete(levels, price)
		} else {
			levels[price] = level
		}
		if (i+1)%1_000 == 0 {
			assertBookAggregates(t, engine, expectedBids, expectedAsks)
		}
	}
	assertBookAggregates(t, engine, expectedBids, expectedAsks)
}

func TestHighFrequencySweepMaintainsPriceTimePriority(t *testing.T) {
	const makerCount = 5_000
	const sweepAmount = 2_500
	engine := matchingEngine()
	makersAt100 := make([]int64, 0, makerCount/2)
	makersAt101 := make([]int64, 0, makerCount/2)

	for i := 0; i < makerCount; i++ {
		price := int64(101)
		if i%3 == 0 {
			price = 100
		}
		events := engine.Apply(&PlaceOrder{
			AccountID:     int64(1 + i),
			ClientOrderID: fmt.Sprintf("maker-%d", i),
			OrderSide:     constant.Sell,
			PositionSide:  constant.Short,
			Type:          constant.Limit,
			Price:         price,
			Amount:        1,
			Leverage:      1,
		})
		accepted, ok := events[0].(*OrderAccepted)
		if !ok {
			t.Fatalf("maker %d event = %T", i, events[0])
		}
		if price == 100 {
			makersAt100 = append(makersAt100, accepted.OrderID)
		} else {
			makersAt101 = append(makersAt101, accepted.OrderID)
		}
	}

	events := engine.Apply(&PlaceOrder{
		AccountID:     10_000,
		ClientOrderID: "sweep",
		OrderSide:     constant.Buy,
		PositionSide:  constant.Long,
		Type:          constant.Limit,
		Price:         101,
		Amount:        sweepAmount,
		Leverage:      1,
	})
	trades := tradeEvents(events)
	if len(trades) != sweepAmount {
		t.Fatalf("trades = %d, want %d", len(trades), sweepAmount)
	}

	expectedMakers := append(append([]int64{}, makersAt100...), makersAt101...)
	previousSeq := int64(0)
	for i, trade := range trades {
		if trade.Seq <= previousSeq {
			t.Fatalf("trade %d sequence = %d, previous = %d", i, trade.Seq, previousSeq)
		}
		previousSeq = trade.Seq

		wantMaker := expectedMakers[i]
		wantPrice := int64(100)
		if wantMaker == 0 {
			t.Fatalf("trade %d has no expected maker", i)
		}
		if i >= len(makersAt100) {
			wantPrice = 101
		}
		if trade.MakerOrderID != wantMaker || trade.Price != wantPrice || trade.Amount != 1 {
			t.Fatalf("trade %d = %+v, want maker %d price %d", i, trade, wantMaker, wantPrice)
		}
		if trade.MakerState != constant.StateFilled || trade.MakerRemaining != 0 {
			t.Fatalf("trade %d maker state = %+v", i, trade)
		}

		wantTakerRemaining := int64(sweepAmount - i - 1)
		wantTakerState := constant.StatePartial
		if wantTakerRemaining == 0 {
			wantTakerState = constant.StateFilled
		}
		if trade.TakerRemaining != wantTakerRemaining || trade.TakerState != wantTakerState {
			t.Fatalf("trade %d taker state = (%s,%d), want (%s,%d)",
				i, trade.TakerState, trade.TakerRemaining, wantTakerState, wantTakerRemaining)
		}
	}

	if order := engine.GetOrder(expectedMakers[len(expectedMakers)-1]); order == nil ||
		order.State != constant.StateInBook || order.Remaining != 1 {
		t.Fatalf("remaining maker = %+v", order)
	}
	price, amount, ok := engine.OrderBook().BestAsk()
	if !ok || price != 101 || amount != int64(len(expectedMakers)-sweepAmount) {
		t.Fatalf("BestAsk = (%d,%d,%v)", price, amount, ok)
	}
	assertUniqueIncreasingSequence(t, events)
}

func assertBookAggregates(
	t *testing.T,
	engine *Engine,
	expectedBids map[int64]orderbook.Level,
	expectedAsks map[int64]orderbook.Level,
) {
	t.Helper()
	assertSideAggregates(t, engine.OrderBook().Bids(100), expectedBids, true)
	assertSideAggregates(t, engine.OrderBook().Asks(100), expectedAsks, false)
}

func assertSideAggregates(t *testing.T, got []orderbook.Level, expected map[int64]orderbook.Level, descending bool) {
	t.Helper()
	if len(got) != len(expected) {
		t.Fatalf("level count = %d, want %d", len(got), len(expected))
	}
	for i, level := range got {
		if i > 0 {
			if descending && got[i-1].Price <= level.Price {
				t.Fatalf("bids not descending: %+v", got)
			}
			if !descending && got[i-1].Price >= level.Price {
				t.Fatalf("asks not ascending: %+v", got)
			}
		}
		want, ok := expected[level.Price]
		if !ok || want != level {
			t.Fatalf("level %d = %+v, want %+v", i, level, want)
		}
	}
}

func positionSideForTest(side constant.OrderSide) constant.PositionSide {
	if side == constant.Buy {
		return constant.Long
	}
	return constant.Short
}
