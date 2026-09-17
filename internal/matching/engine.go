package matching

import (
	"context"
	"my-go-exchange/internal/errs"
	"my-go-exchange/internal/orderbook"
	"my-go-exchange/internal/types"
)

// Engine 接收 command 撮合并返回 event
// Seq / OrderID / TradeID 只在这里分配，OrderBook 不再改写 Seq
type Engine struct {
	inst *types.Instrument
	ob   *orderbook.OrderBook

	seq     uint64
	orderID uint64
	tradeID uint64
}

func NewEngine(inst *types.Instrument) *Engine {
	return &Engine{
		inst: inst,
		ob:   orderbook.NewOrderBook(inst),
	}
}

func (e *Engine) OrderBook() *orderbook.OrderBook {
	return e.ob
}

func (e *Engine) Apply(cmd Command) []Event {
	switch c := cmd.(type) {
	case *PlaceOrder:
		return e.applyPlace(c)
	case *CancelOrder:
		return e.applyCancel(c)
	default:
		return nil
	}
}

func (e *Engine) applyPlace(cmd *PlaceOrder) []Event {
	e.seq++

	if err := orderbook.ValidatePlace(cmd.OrderType, cmd.Price, cmd.Amount, cmd.Side, cmd.TIF); err != nil {
		return []Event{
			&OrderRejected{
				Seq:    e.seq,
				Reason: err.Error(),
			},
		}
	}

	// FOK：先预检能否全部成交
	if cmd.TIF == types.FOK {
		avail := e.ob.Available(cmd.Side, cmd.Price, cmd.OrderType == types.Market)
		if avail < cmd.Amount {
			return []Event{
				&OrderRejected{
					Seq:    e.seq,
					Reason: errs.ErrFOKCannotFill.Error(),
				},
			}
		}
	}

	e.orderID++
	taker := &types.Order{
		ID:        e.orderID,
		AccountID: cmd.AccountID,
		Side:      cmd.Side,
		Type:      cmd.OrderType,
		Price:     cmd.Price,
		Amount:    cmd.Amount,
		Remaining: cmd.Amount,
		TIF:       cmd.TIF,
		Seq:       e.seq,
		State:     types.StateAccept,
	}

	events := []Event{
		&OrderAccepted{
			Seq:       e.seq,
			OrderID:   taker.ID,
			AccountID: taker.AccountID,
			Side:      taker.Side,
			Price:     taker.Price,
			OrderType: taker.Type,
			Amount:    taker.Amount,
			TIF:       taker.TIF,
		},
	}

	// 撮合
	for taker.Remaining > 0 {
		opposite := taker.Side.Opposite()
		best, _, ok := e.best(opposite)
		if !ok || !e.canCross(taker, best) {
			break
		}
		maker, ok := e.ob.Front(opposite)
		if !ok {
			break
		}

		q := taker.Remaining
		if q > maker.Remaining {
			q = maker.Remaining
		}
		e.tradeID++
		events = append(events, &Trade{
			Seq:            e.seq,
			TradeID:        e.tradeID,
			Price:          maker.Price,
			Amount:         q,
			TakerSide:      taker.Side,
			TakerOrderID:   taker.ID,
			MakerOrderID:   maker.ID,
			TakerAccountID: taker.AccountID,
			MakerAccountID: maker.AccountID,
		})
		taker.Remaining -= q
		if err := e.ob.Fill(maker.ID, q); err != nil {
			panic("Fill failed: " + err.Error())
		}
	}

	return e.finishTaker(taker, events)
}

// finishTaker 按 TIF / OrderType 处理剩余量
func (e *Engine) finishTaker(taker *types.Order, events []Event) []Event {
	if taker.Remaining == 0 {
		taker.State = types.StateFilled
		return events
	}

	switch taker.TIF {
	case types.IOC:
		taker.State = types.StateCanceled
		events = append(events, &OrderCancelled{
			Seq:       e.seq,
			OrderID:   taker.ID,
			Remaining: taker.Remaining,
		})
	case types.FOK:
		// 预检通过后理论上应全部成交；防御：不应走到这里
		taker.State = types.StateCanceled
		events = append(events, &OrderCancelled{
			Seq:       e.seq,
			OrderID:   taker.ID,
			Remaining: taker.Remaining,
		})
	case types.GTC:
		// 只有限价 GTC 才允许挂入 book
		if taker.Type != types.Limit {
			taker.State = types.StateCanceled
			events = append(events, &OrderCancelled{
				Seq:       e.seq,
				OrderID:   taker.ID,
				Remaining: taker.Remaining,
			})
			return events
		}
		if taker.Amount == taker.Remaining {
			taker.State = types.StateInBook
		} else {
			taker.State = types.StatePartial
		}
		if err := e.ob.Place(taker); err != nil {
			// 不 panic，退回 rejected
			return append(events, &OrderRejected{
				Seq:    e.seq,
				Reason: err.Error(),
			})
		}
	}
	return events
}

func (e *Engine) applyCancel(cmd *CancelOrder) []Event {
	e.seq++

	order, err := e.ob.Cancel(cmd.OrderID)
	if err != nil {
		return []Event{
			&OrderRejected{
				Seq:    e.seq,
				Reason: err.Error(),
			},
		}
	}
	return []Event{
		&OrderCancelled{
			Seq:       e.seq,
			OrderID:   order.ID,
			Remaining: order.Remaining,
		},
	}
}

func (e *Engine) best(side types.Side) (price uint64, totalAmount uint64, ok bool) {
	if side == types.Buy {
		return e.ob.BestBid()
	}
	return e.ob.BestAsk()
}

// canCross 市价单始终可交叉；限价单按价格判断
func (e *Engine) canCross(taker *types.Order, oppositePrice uint64) bool {
	if taker.Type == types.Market {
		return true
	}
	if taker.Side == types.Buy {
		return oppositePrice <= taker.Price
	}
	return oppositePrice >= taker.Price
}

func RunMatchingEngine(ctx context.Context, engine *Engine, in <-chan Command, out chan<- Event) error {
	for {
		select {
		case <-ctx.Done():
			return nil
		case cmd := <-in:
			events := engine.Apply(cmd)
			for _, event := range events {
				select {
				case <-ctx.Done():
					return nil
				case out <- event:
				}
			}
		}
	}
}
