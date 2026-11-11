package matching

import (
	"context"
	"fmt"

	"my-go-exchange/internal/errs"
	"my-go-exchange/internal/orderbook"
	"my-go-exchange/internal/types"
)

// Engine applies commands to one instrument's order book and returns all
// resulting events in causal order. Apply is not safe for concurrent use.
type Engine struct {
	inst *types.Instrument
	ob   *orderbook.OrderBook

	orders  map[uint64]*types.Order
	seq     uint64
	orderID uint64
	tradeID uint64
}

func NewEngine(inst *types.Instrument) *Engine {
	return &Engine{
		inst:   inst,
		ob:     orderbook.NewOrderBook(inst),
		orders: make(map[uint64]*types.Order),
	}
}

// OrderBook exposes the engine's current book for market-data adapters and
// tests. Callers must not mutate orders through this module.
func (e *Engine) OrderBook() *orderbook.OrderBook {
	return e.ob
}

// Apply executes one command and returns its ordered events.
func (e *Engine) Apply(cmd Command) []Event {
	switch command := cmd.(type) {
	case *PlaceOrder:
		if command == nil {
			return e.reject(0, "nil place-order command")
		}
		return e.applyPlace(command)
	case *CancelOrder:
		if command == nil {
			return e.reject(0, "nil cancel-order command")
		}
		return e.applyCancel(command)
	default:
		return e.reject(0, fmt.Sprintf("unsupported command type %T", cmd))
	}
}

func (e *Engine) applyPlace(cmd *PlaceOrder) []Event {
	if cmd.AccountID == 0 {
		return e.reject(0, errs.ErrInvalidAccountID.Error())
	}
	if err := orderbook.ValidatePlace(cmd.Type, cmd.Price, cmd.Amount, cmd.Side, cmd.TIF); err != nil {
		return e.reject(0, err.Error())
	}

	// TIF/FOK preflight is intentionally disabled until TIF semantics are defined.
	// if cmd.TIF == types.FOK {
	// 	market := cmd.Type == types.Market
	// 	if available := e.ob.Available(cmd.Side, cmd.Price, market); available < cmd.Amount {
	// 		return e.reject(0, errs.ErrFOKCannotFill.Error())
	// 	}
	// }

	e.orderID++
	acceptedSeq := e.nextSequence()
	taker := &types.Order{
		ID:        e.orderID,
		AccountID: cmd.AccountID,
		Side:      cmd.Side,
		Type:      cmd.Type,
		Price:     cmd.Price,
		Amount:    cmd.Amount,
		Remaining: cmd.Amount,
		TIF:       cmd.TIF,
		Seq:       acceptedSeq,
		State:     types.StateAccept,
	}

	e.orders[taker.ID] = taker

	events := []Event{
		&OrderAccepted{
			Seq:       acceptedSeq,
			OrderID:   taker.ID,
			AccountID: taker.AccountID,
			Price:     taker.Price,
			Amount:    taker.Amount,
			Side:      taker.Side,
			Type:      taker.Type,
			TIF:       taker.TIF,
		},
	}

	events = append(events, e.match(taker)...)
	return e.finishTaker(taker, events)
}

func (e *Engine) applyCancel(cmd *CancelOrder) []Event {
	if cmd.OrderID == 0 {
		return e.reject(0, errs.ErrInvalidOrderID.Error())
	}

	order, known := e.orders[cmd.OrderID]
	if !known {
		return e.reject(cmd.OrderID, errs.ErrOrderNotFound.Error())
	}
	if order.State.IsTerminal() {
		return e.reject(cmd.OrderID, errs.ErrOrderNotCancellable.Error())
	}

	remaining, err := e.ob.Cancel(cmd.OrderID)
	if err != nil {
		return e.reject(cmd.OrderID, err.Error())
	}

	return []Event{
		&OrderCanceled{
			Seq:       e.nextSequence(),
			OrderID:   cmd.OrderID,
			Remaining: remaining,
		},
	}
}

// match consumes price/time-priority liquidity until the taker is filled or no
// eligible resting order remains.
func (e *Engine) match(taker *types.Order) []Event {
	events := make([]Event, 0)

	for taker.Remaining > 0 {
		maker, ok := e.ob.Front(taker.Side.Opposite())
		if !ok || !e.crosses(taker, maker.Price) {
			break
		}

		amount := taker.Remaining
		if amount > maker.Remaining {
			amount = maker.Remaining
		}
		if err := e.ob.Fill(maker.ID, amount); err != nil {
			break
		}

		taker.Remaining -= amount
		e.tradeID++
		events = append(events, &Trade{
			Seq:            e.nextSequence(),
			TradeID:        e.tradeID,
			Price:          maker.Price,
			Amount:         amount,
			TakerSide:      taker.Side,
			TakerOrderID:   taker.ID,
			MakerOrderID:   maker.ID,
			TakerAccountID: taker.AccountID,
			MakerAccountID: maker.AccountID,
		})
	}

	return events
}

// finishTaker applies time-in-force and order-type semantics to any quantity
// that did not execute.
func (e *Engine) finishTaker(taker *types.Order, events []Event) []Event {
	if taker.Remaining == 0 {
		taker.State = types.StateFilled
		return events
	}

	// Market orders never rest. TIF-specific remainder handling is disabled.
	if taker.Type == types.Market {
		taker.State = types.StateCanceled
		return append(events, &OrderCanceled{
			Seq:       e.nextSequence(),
			OrderID:   taker.ID,
			Remaining: taker.Remaining,
		})
	}

	if err := e.ob.Place(taker); err != nil {
		taker.State = types.StateCanceled
		return append(events, &OrderCanceled{
			Seq:       e.nextSequence(),
			OrderID:   taker.ID,
			Remaining: taker.Remaining,
		})
	}
	return events
}

func (e *Engine) crosses(taker *types.Order, oppositePrice uint64) bool {
	if taker.Type == types.Market {
		return true
	}
	if taker.Side == types.Buy {
		return taker.Price >= oppositePrice
	}
	return taker.Price <= oppositePrice
}

func (e *Engine) reject(orderID uint64, reason string) []Event {
	return []Event{
		&OrderRejected{
			Seq:     e.nextSequence(),
			OrderID: orderID,
			Reason:  reason,
		},
	}
}

func (e *Engine) nextSequence() uint64 {
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
