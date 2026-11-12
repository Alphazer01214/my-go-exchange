package matching

import (
	"context"
	"my-go-exchange/internal/trading"
	"strings"
	"testing"
	"time"

	"my-go-exchange/internal/constant"
)

func matchingEngine() *Engine {
	return NewEngine(&trading.Instrument{Symbol: "TEST/QUOTE"}, NewIDAllocator())
}

func place(
	accountID int64,
	side constant.OrderSide,
	orderType constant.OrderType,
	price int64,
	amount int64,
) *PlaceOrder {
	return &PlaceOrder{
		AccountID: accountID,
		OrderSide: side,
		Type:      orderType,
		Price:     price,
		Amount:    amount,
		// TIF:       tif,
	}
}

func tradeEvents(events []Event) []*TradeEvent {
	trades := make([]*TradeEvent, 0, len(events))
	for _, event := range events {
		if trade, ok := event.(*TradeEvent); ok {
			trades = append(trades, trade)
		}
	}
	return trades
}

func assertUniqueIncreasingSequence(t *testing.T, events []Event) {
	t.Helper()
	previous := int64(0)
	for _, event := range events {
		if event.Sequence() <= previous {
			t.Fatalf("event sequence = %d, previous = %d", event.Sequence(), previous)
		}
		previous = event.Sequence()
	}
}

func TestLimitRestsWithoutCrossing(t *testing.T) {
	engine := matchingEngine()
	events := engine.Apply(&PlaceOrder{
		AccountID:     1,
		ClientOrderID: "client-1",
		OrderSide:     constant.Buy,
		PositionSide:  constant.Long,
		Type:          constant.Limit,
		Price:         100,
		Amount:        10,
		Leverage:      5,
	})

	if len(events) != 1 {
		t.Fatalf("events = %+v, want one accepted event", events)
	}
	accepted, ok := events[0].(*OrderAccepted)
	if !ok {
		t.Fatalf("event type = %T, want OrderAccepted", events[0])
	}
	if accepted.Seq != 1 || accepted.OrderID != 1 {
		t.Fatalf("accepted = %+v, want seq 1 and order 1", accepted)
	}
	if accepted.ClientOrderID != "client-1" || accepted.Symbol != "TEST/QUOTE" ||
		accepted.PositionSide != constant.Long || accepted.Leverage != 5 {
		t.Fatalf("accepted identity = %+v", accepted)
	}
	if order := engine.GetOrder(accepted.OrderID); order == nil ||
		order.ClientOrderID != "client-1" || order.State != constant.StateInBook {
		t.Fatalf("resting order = %+v, want client-1 and inbook", order)
	}

	price, amount, ok := engine.OrderBook().BestBid()
	if !ok || price != 100 || amount != 10 {
		t.Fatalf("BestBid = (%d,%d,%v), want (100,10,true)", price, amount, ok)
	}
}

func TestApplyWithResultCapturesBookChanges(t *testing.T) {
	engine := matchingEngine()
	result := engine.ApplyWithResult(place(1, constant.Buy, constant.Limit, 100, 10))
	if len(result.Events) != 1 {
		t.Fatalf("events = %+v, want one accepted event", result.Events)
	}
	if len(result.BookChanges.Bids) != 1 {
		t.Fatalf("bid changes = %+v, want one level", result.BookChanges.Bids)
	}
	if change := result.BookChanges.Bids[0]; change.Price != 100 || change.Amount != 10 {
		t.Fatalf("bid change = %+v, want price 100 amount 10", change)
	}
}

func TestPartialFillThenRest(t *testing.T) {
	engine := matchingEngine()
	engine.Apply(place(1, constant.Sell, constant.Limit, 100, 4))

	events := engine.Apply(place(2, constant.Buy, constant.Limit, 100, 10))
	trades := tradeEvents(events)
	if len(trades) != 1 {
		t.Fatalf("trades = %+v, want one trade", trades)
	}
	trade := trades[0]
	if trade.Seq != 3 || trade.TradeID != 1 {
		t.Fatalf("trade identity = (%d,%d), want (3,1)", trade.Seq, trade.TradeID)
	}
	if trade.Price != 100 || trade.Amount != 4 {
		t.Fatalf("trade execution = (%d,%d), want (100,4)", trade.Price, trade.Amount)
	}
	if trade.MakerOrderID != 1 || trade.TakerOrderID != 2 {
		t.Fatalf("trade orders = (%d,%d), want (1,2)", trade.MakerOrderID, trade.TakerOrderID)
	}
	if trade.MakerAccountID != 1 || trade.TakerAccountID != 2 {
		t.Fatalf("trade accounts = (%d,%d), want (1,2)", trade.MakerAccountID, trade.TakerAccountID)
	}
	if trade.MakerState != constant.StateFilled || trade.MakerRemaining != 0 {
		t.Fatalf("maker post-fill = (%s,%d), want filled/0", trade.MakerState, trade.MakerRemaining)
	}
	if trade.TakerState != constant.StatePartial || trade.TakerRemaining != 6 {
		t.Fatalf("taker post-fill = (%s,%d), want partial/6", trade.TakerState, trade.TakerRemaining)
	}
	assertUniqueIncreasingSequence(t, events)

	resting, ok := engine.OrderBook().Front(constant.Buy)
	if !ok || resting.ID != 2 || resting.Remaining != 6 || resting.Seq != 2 {
		t.Fatalf("resting taker = %+v, want order 2 remaining 6 at seq 2", resting)
	}
	maker := engine.GetOrder(1)
	if maker == nil || maker.State != constant.StateFilled || maker.Remaining != 0 {
		t.Fatalf("maker = %+v, want filled with zero remaining", maker)
	}
	taker := engine.GetOrder(2)
	if taker == nil || taker.State != constant.StatePartial || taker.Remaining != 6 {
		t.Fatalf("taker = %+v, want partial with six remaining", taker)
	}
	price, amount, ok := engine.OrderBook().BestBid()
	if !ok || price != 100 || amount != 6 {
		t.Fatalf("BestBid = (%d,%d,%v), want (100,6,true)", price, amount, ok)
	}
}

func TestMatchingUsesPriceThenTimePriority(t *testing.T) {
	engine := matchingEngine()
	engine.Apply(place(1, constant.Sell, constant.Limit, 100, 2))
	engine.Apply(place(2, constant.Sell, constant.Limit, 100, 3))
	engine.Apply(place(3, constant.Sell, constant.Limit, 101, 5))

	events := engine.Apply(place(4, constant.Buy, constant.Limit, 101, 4))
	trades := tradeEvents(events)
	if len(trades) != 2 {
		t.Fatalf("trades = %+v, want two trades", trades)
	}
	if trades[0].MakerOrderID != 1 || trades[0].Amount != 2 {
		t.Fatalf("first trade = %+v, want maker 1 amount 2", trades[0])
	}
	if trades[1].MakerOrderID != 2 || trades[1].Amount != 2 {
		t.Fatalf("second trade = %+v, want maker 2 amount 2", trades[1])
	}

	maker, ok := engine.OrderBook().Front(constant.Sell)
	if !ok || maker.ID != 2 || maker.Remaining != 1 {
		t.Fatalf("remaining best maker = %+v, want order 2 remaining 1", maker)
	}
	if makerState := engine.GetOrder(2); makerState == nil ||
		makerState.State != constant.StatePartial {
		t.Fatalf("maker state = %+v, want partial", makerState)
	}
	if takerState := engine.GetOrder(4); takerState == nil ||
		takerState.State != constant.StateFilled {
		t.Fatalf("taker state = %+v, want filled", takerState)
	}
}

func TestOrderBookSharesEngineOrderPointer(t *testing.T) {
	engine := matchingEngine()
	engine.Apply(place(1, constant.Buy, constant.Limit, 100, 1))

	order := engine.GetOrder(1)
	front, ok := engine.OrderBook().Front(constant.Buy)
	if !ok || order != front {
		t.Fatalf("front = %p, want engine order %p", front, order)
	}
}

func TestMarketSweepsLevelsAndCancelsRemainder(t *testing.T) {
	engine := matchingEngine()
	engine.Apply(place(1, constant.Sell, constant.Limit, 100, 2))
	engine.Apply(place(2, constant.Sell, constant.Limit, 110, 3))

	events := engine.Apply(place(3, constant.Buy, constant.Market, 0, 10))
	trades := tradeEvents(events)
	if len(trades) != 2 {
		t.Fatalf("trades = %+v, want two trades", trades)
	}
	if trades[0].Price != 100 || trades[0].Amount != 2 {
		t.Fatalf("first trade = %+v, want price 100 amount 2", trades[0])
	}
	if trades[1].Price != 110 || trades[1].Amount != 3 {
		t.Fatalf("second trade = %+v, want price 110 amount 3", trades[1])
	}

	canceled, ok := events[len(events)-1].(*OrderCanceled)
	if !ok || canceled.OrderID != 3 || canceled.Remaining != 5 ||
		canceled.Reason != "market_order_unfilled" {
		t.Fatalf("last event = %+v, want canceled order 3 remaining 5", events[len(events)-1])
	}
	if _, _, ok := engine.OrderBook().BestBid(); ok {
		t.Fatal("market remainder must not rest")
	}
	assertUniqueIncreasingSequence(t, events)
}

// func TestIOCPartiallyFillsThenCancels(t *testing.T) {
// 	engine := matchingEngine()
// 	engine.Apply(place(1, types.Sell, types.Limit, 100, 2, types.GTC))
//
// 	events := engine.Apply(place(2, types.Buy, types.Limit, 100, 10, types.IOC))
// 	if trades := tradeEvents(events); len(trades) != 1 || trades[0].Amount != 2 {
// 		t.Fatalf("trades = %+v, want one trade of 2", trades)
// 	}
//
// 	canceled, ok := events[len(events)-1].(*OrderCanceled)
// 	if !ok || canceled.Remaining != 8 {
// 		t.Fatalf("last event = %+v, want remaining 8", events[len(events)-1])
// 	}
// 	if _, _, ok := engine.OrderBook().BestBid(); ok {
// 		t.Fatal("IOC remainder must not rest")
// 	}
// }
//
// func TestFOKRejectsAtomicallyWhenLiquidityIsInsufficient(t *testing.T) {
// 	engine := matchingEngine()
// 	engine.Apply(place(1, types.Sell, types.Limit, 100, 5, types.GTC))
//
// 	events := engine.Apply(place(2, types.Buy, types.Limit, 100, 6, types.FOK))
// 	if len(events) != 1 {
// 		t.Fatalf("events = %+v, want one rejection", events)
// 	}
// 	rejected, ok := events[0].(*OrderRejected)
// 	if !ok {
// 		t.Fatalf("event type = %T, want OrderRejected", events[0])
// 	}
// 	if rejected.OrderID != 0 || rejected.Reason != errs.ErrFOKCannotFill.Error() {
// 		t.Fatalf("rejection = %+v", rejected)
// 	}
// 	if len(tradeEvents(events)) != 0 {
// 		t.Fatal("rejected FOK must not trade")
// 	}
//
// 	if _, _, ok := engine.OrderBook().BestAsk(); !ok {
// 		t.Fatal("maker should remain in book")
// 	}
// }
//
// func TestFOKCanFillAcrossPriceLevels(t *testing.T) {
// 	engine := matchingEngine()
// 	engine.Apply(place(1, types.Sell, types.Limit, 100, 2, types.GTC))
// 	engine.Apply(place(2, types.Sell, types.Limit, 101, 3, types.GTC))
//
// 	events := engine.Apply(place(3, types.Buy, types.Limit, 101, 5, types.FOK))
// 	trades := tradeEvents(events)
// 	if len(trades) != 2 {
// 		t.Fatalf("trades = %+v, want two trades", trades)
// 	}
// 	if trades[0].Amount != 2 || trades[1].Amount != 3 {
// 		t.Fatalf("trade amounts = (%d,%d), want (2,3)", trades[0].Amount, trades[1].Amount)
// 	}
// 	if _, _, ok := engine.OrderBook().BestBid(); ok {
// 		t.Fatal("completed FOK must not rest")
// 	}
// }

func TestCancelRestingOrderAndRejectUnknownOrder(t *testing.T) {
	engine := matchingEngine()
	engine.Apply(place(1, constant.Buy, constant.Limit, 100, 10))

	canceledEvents := engine.Apply(&CancelOrder{OrderID: 1})
	canceled, ok := canceledEvents[0].(*OrderCanceled)
	if !ok || canceled.Remaining != 10 || canceled.Reason != "user_canceled" {
		t.Fatalf("cancel events = %+v", canceledEvents)
	}
	if order := engine.GetOrder(1); order == nil ||
		order.State != constant.StateCanceled {
		t.Fatalf("canceled order = %+v, want canceled", order)
	}
	if _, _, ok := engine.OrderBook().BestBid(); ok {
		t.Fatal("canceled bid should be removed")
	}

	rejectedEvents := engine.Apply(&CancelOrder{OrderID: 1})
	rejected, ok := rejectedEvents[0].(*OrderRejected)
	if !ok || rejected.OrderID != 1 || rejected.Reason != constant.ErrOrderNotCancellable.Error() {
		t.Fatalf("unknown-order events = %+v", rejectedEvents)
	}
}

func TestInvalidPlaceIsRejectedBeforeAcceptance(t *testing.T) {
	engine := matchingEngine()
	tests := []*PlaceOrder{
		place(0, constant.Buy, constant.Limit, 100, 1),
		place(1, constant.Buy, constant.Limit, 0, 1),
		place(1, constant.Buy, constant.Market, 100, 1),
	}

	for _, command := range tests {
		command.ClientOrderID = "client-invalid"
		events := engine.Apply(command)
		if len(events) != 1 {
			t.Fatalf("events = %+v, want one rejection", events)
		}
		rejected, ok := events[0].(*OrderRejected)
		if !ok {
			t.Fatalf("event type = %T, want OrderRejected", events[0])
		}
		if rejected.AccountID != command.AccountID ||
			rejected.ClientOrderID != command.ClientOrderID ||
			rejected.Reason == "" {
			t.Fatalf("rejection identity = %+v", rejected)
		}
	}

	if _, _, ok := engine.OrderBook().BestBid(); ok {
		t.Fatal("rejected orders must not enter the book")
	}
}

func TestUnsupportedAndNilCommandsAreRejected(t *testing.T) {
	engine := matchingEngine()
	for _, command := range []Command{nil, (*PlaceOrder)(nil), &unsupportedCommand{}} {
		events := engine.Apply(command)
		if len(events) != 1 {
			t.Fatalf("events = %+v, want one rejection", events)
		}
		rejected, ok := events[0].(*OrderRejected)
		if !ok || !strings.Contains(rejected.Reason, "command") {
			t.Fatalf("events = %+v, want command rejection", events)
		}
	}
}

type unsupportedCommand struct{}

func (*unsupportedCommand) Kind() CommandType { return CommandType("unsupported") }

func TestRunMatchingEngineSerializesCommands(t *testing.T) {
	engine := matchingEngine()
	in := make(chan Command, 2)
	out := make(chan Event, 4)
	in <- place(1, constant.Buy, constant.Limit, 100, 1)
	in <- &CancelOrder{OrderID: 1}
	close(in)

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := RunMatchingEngine(ctx, engine, in, out); err != nil {
		t.Fatal(err)
	}
	close(out)

	var events []Event
	for event := range out {
		events = append(events, event)
	}
	if len(events) != 2 {
		t.Fatalf("events = %+v, want accepted and canceled", events)
	}
	if _, ok := events[0].(*OrderAccepted); !ok {
		t.Fatalf("first event type = %T", events[0])
	}
	if _, ok := events[1].(*OrderCanceled); !ok {
		t.Fatalf("second event type = %T", events[1])
	}
	assertUniqueIncreasingSequence(t, events)
}

func TestRunMatchingEngineRejectsNilEngine(t *testing.T) {
	err := RunMatchingEngine(context.Background(), nil, nil, nil)
	if err == nil || !strings.Contains(err.Error(), "nil") {
		t.Fatalf("error = %v, want nil-engine error", err)
	}
}
