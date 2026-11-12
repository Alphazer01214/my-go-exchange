package matching

import (
	"context"
	"fmt"
	"my-go-exchange/internal/trading"

	"my-go-exchange/internal/constant"
	"my-go-exchange/internal/orderbook"
)

// Engine applies commands to one instrument's order book and returns all
// resulting events in causal order. Apply is not safe for concurrent use.
type Engine struct {
	inst *trading.Instrument
	ob   *orderbook.OrderBook

	orders map[int64]*trading.Order
	ids    *IDAllocator
	seq    int64
}

// ApplyResult contains every state change caused by a command. Events preserve
// matching causality, while BookChanges contains the final state of each
// affected price level.
type ApplyResult struct {
	Events      []Event
	BookChanges orderbook.ChangeSet
}

// Merge appends another result while preserving event order and replacing
// duplicated price-level changes with their latest values.
func (r *ApplyResult) Merge(other ApplyResult) {
	if r == nil {
		return
	}
	r.Events = append(r.Events, other.Events...)
	r.BookChanges.Merge(other.BookChanges)
}

func NewEngine(inst *trading.Instrument, ids *IDAllocator) *Engine {
	if ids == nil {
		panic("matching: ID allocator is required")
	}
	return &Engine{
		inst:   inst,
		ob:     orderbook.NewOrderBook(inst),
		orders: make(map[int64]*trading.Order),
		ids:    ids,
	}
}

// OrderBook exposes the engine's current book for market-data adapters and
// tests. Callers outside the matching module must not mutate its orders.
func (e *Engine) OrderBook() *orderbook.OrderBook {
	return e.ob
}

// GetOrder returns the engine-owned order pointer used by matching.
func (e *Engine) GetOrder(orderID int64) *trading.Order {
	return e.orders[orderID]
}

// Apply executes one command and returns its ordered events.
func (e *Engine) Apply(cmd Command) []Event {
	return e.ApplyWithResult(cmd).Events
}

// ApplyWithResult executes one command and includes its order-book side
// effects.
func (e *Engine) ApplyWithResult(cmd Command) ApplyResult {
	e.ob.BeginTracking()
	switch command := cmd.(type) {
	case *PlaceOrder:
		if command == nil {
			return ApplyResult{
				Events:      e.reject(0, 0, "", "nil place-order command"),
				BookChanges: e.ob.EndTracking(),
			}
		}
		return ApplyResult{
			Events:      e.applyPlace(command),
			BookChanges: e.ob.EndTracking(),
		}
	case *CancelOrder:
		if command == nil {
			return ApplyResult{
				Events:      e.reject(0, 0, "", "nil cancel-order command"),
				BookChanges: e.ob.EndTracking(),
			}
		}
		return ApplyResult{
			Events:      e.applyCancel(command),
			BookChanges: e.ob.EndTracking(),
		}
	default:
		return ApplyResult{
			Events:      e.reject(0, 0, "", fmt.Sprintf("unsupported command type %T", cmd)),
			BookChanges: e.ob.EndTracking(),
		}
	}
}

func (e *Engine) applyPlace(cmd *PlaceOrder) []Event {
	if cmd.AccountID <= 0 {
		return e.reject(0, cmd.AccountID, cmd.ClientOrderID, constant.ErrInvalidAccountID.Error())
	}
	if err := orderbook.ValidatePlace(cmd.Type, cmd.Price, cmd.Amount, cmd.OrderSide); err != nil {
		return e.reject(0, cmd.AccountID, cmd.ClientOrderID, err.Error())
	}

	// TIF/FOK preflight is intentionally disabled until TIF semantics are defined.
	// if cmd.TIF == constant.FOK {
	// 	market := cmd.Type == constant.Market
	// 	if available := e.ob.Available(cmd.Side, cmd.Price, market); available < cmd.Amount {
	// 		return e.reject(0, constant.ErrFOKCannotFill.Error())
	// 	}
	// }

	acceptedSeq := e.nextSequence()
	taker := &trading.Order{
		ID:            e.ids.nextOrderID(),
		AccountID:     cmd.AccountID,
		ClientOrderID: cmd.ClientOrderID,
		Symbol:        e.inst.Symbol,
		OrderSide:     cmd.OrderSide,
		PositionSide:  cmd.PositionSide,
		Type:          cmd.Type,
		Price:         cmd.Price,
		Amount:        cmd.Amount,
		Leverage:      cmd.Leverage,
		Remaining:     cmd.Amount,
		// TIF:       cmd.TIF,
		Seq:   acceptedSeq,
		State: constant.StateAccept,
	}

	e.orders[taker.ID] = taker

	events := []Event{
		&OrderAccepted{
			Seq:           acceptedSeq,
			OrderID:       taker.ID,
			AccountID:     taker.AccountID,
			ClientOrderID: taker.ClientOrderID,
			Symbol:        taker.Symbol,
			Price:         taker.Price,
			Amount:        taker.Amount,
			OrderSide:     taker.OrderSide,
			PositionSide:  taker.PositionSide,
			Type:          taker.Type,
			Leverage:      taker.Leverage,
			// TIF:       taker.TIF,
		},
	}

	events = append(events, e.match(taker)...)
	return e.finishTaker(taker, events)
}

func (e *Engine) applyCancel(cmd *CancelOrder) []Event {
	if cmd.OrderID <= 0 {
		return e.reject(cmd.OrderID, 0, "", constant.ErrInvalidOrderID.Error())
	}

	order, known := e.orders[cmd.OrderID]
	if !known {
		return e.reject(cmd.OrderID, 0, "", constant.ErrOrderNotFound.Error())
	}
	if order.State.IsTerminal() {
		return e.reject(cmd.OrderID, order.AccountID, order.ClientOrderID, constant.ErrOrderNotCancellable.Error())
	}

	remaining, err := e.ob.Remove(cmd.OrderID)
	if err != nil {
		return e.reject(cmd.OrderID, order.AccountID, order.ClientOrderID, err.Error())
	}
	order.State = constant.StateCanceled

	return []Event{
		&OrderCanceled{
			Seq:       e.nextSequence(),
			OrderID:   cmd.OrderID,
			Remaining: remaining,
			Reason:    "user_canceled",
		},
	}
}

// match consumes price/time-priority liquidity until the taker is filled or no
// eligible resting order remains.
func (e *Engine) match(taker *trading.Order) []Event {
	events := make([]Event, 0)

	for taker.Remaining > 0 {
		maker, ok := e.ob.Front(taker.OrderSide.Opposite())
		if !ok || !e.crosses(taker, maker.Price) {
			break
		}

		amount := taker.Remaining
		if amount > maker.Remaining {
			amount = maker.Remaining
		}
		if err := e.ob.Consume(maker.ID, amount); err != nil {
			break
		}

		e.applyFill(maker, amount)
		e.applyFill(taker, amount)
		events = append(events, &TradeEvent{
			Seq:            e.nextSequence(),
			TradeID:        e.ids.nextTradeID(),
			Price:          maker.Price,
			Amount:         amount,
			TakerSide:      taker.OrderSide,
			TakerOrderID:   taker.ID,
			MakerOrderID:   maker.ID,
			TakerAccountID: taker.AccountID,
			MakerAccountID: maker.AccountID,
			MakerState:     maker.State,
			MakerRemaining: maker.Remaining,
			TakerState:     taker.State,
			TakerRemaining: taker.Remaining,
		})
	}

	return events
}

// applyFill is the only place that changes an order's Remaining and State for
// executions. OrderBook only maintains the active-book aggregates.
func (e *Engine) applyFill(order *trading.Order, amount int64) {
	order.Remaining -= amount
	if order.Remaining == 0 {
		order.State = constant.StateFilled
		return
	}
	order.State = constant.StatePartial
}

// finishTaker applies time-in-force and order-type semantics to any quantity
// that did not execute.
func (e *Engine) finishTaker(taker *trading.Order, events []Event) []Event {
	if taker.Remaining == 0 {
		taker.State = constant.StateFilled
		return events
	}

	// Market orders never rest. TIF-specific remainder handling is disabled.
	if taker.Type == constant.Market {
		taker.State = constant.StateCanceled
		return append(events, &OrderCanceled{
			Seq:       e.nextSequence(),
			OrderID:   taker.ID,
			Remaining: taker.Remaining,
			Reason:    "market_order_unfilled",
		})
	}

	if taker.Remaining == taker.Amount {
		taker.State = constant.StateInBook
	} else {
		taker.State = constant.StatePartial
	}

	if err := e.ob.Rest(taker); err != nil {
		taker.State = constant.StateCanceled
		return append(events, &OrderCanceled{
			Seq:       e.nextSequence(),
			OrderID:   taker.ID,
			Remaining: taker.Remaining,
			Reason:    "orderbook_rejected",
		})
	}
	return events
}

func (e *Engine) crosses(taker *trading.Order, oppositePrice int64) bool {
	if taker.Type == constant.Market {
		return true
	}
	if taker.OrderSide == constant.Buy {
		return taker.Price >= oppositePrice
	}
	return taker.Price <= oppositePrice
}

func (e *Engine) reject(orderID, accountID int64, clientOrderID, reason string) []Event {
	return []Event{
		&OrderRejected{
			Seq:           e.nextSequence(),
			OrderID:       orderID,
			AccountID:     accountID,
			ClientOrderID: clientOrderID,
			Reason:        reason,
		},
	}
}

func (e *Engine) nextSequence() int64 {
	e.seq++
	return e.seq
}

// RunMatchingEngine serially applies commands and publishes their events until
// the context is canceled or the input channel closes.
func RunMatchingEngine(
	ctx context.Context,
	engine *Engine,
	in <-chan Command,
	out chan<- Event,
) error {
	if engine == nil {
		return fmt.Errorf("matching engine is nil")
	}

	for {
		select {
		case <-ctx.Done():
			return nil
		case command, ok := <-in:
			if !ok {
				return nil
			}
			for _, event := range engine.Apply(command) {
				select {
				case <-ctx.Done():
					return nil
				case out <- event:
				}
			}
		}
	}
}
