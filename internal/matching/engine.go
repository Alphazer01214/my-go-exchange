package matching

import (
	"my-go-exchange/internal/orderbook"
	"my-go-exchange/internal/types"
)

// Engine 接收 command 撮合并返回 event
type Engine struct {
	inst *types.Instrument
	ob   *orderbook.OrderBook

	seq     uint64
	orderID uint64 // 不一定成交的
	tradeID uint64 // 成交的
}

func (e *Engine) applyPlace(cmd PlaceOrder) []Event {
	e.seq++
	e.orderID++
	// cmd -> order -> event -> book

	// taker: 主动成交者
	taker := &types.Order{
		ID:        e.orderID,
		Side:      cmd.Side,
		Type:      cmd.OrderType,
		Price:     cmd.Price,
		Amount:    cmd.Amount,
		Remaining: cmd.Amount,
		TIF:       cmd.TIF,
		Seq:       e.seq,
	}

	events := []Event{
		&OrderAccepted{
			Seq:       e.seq,
			OrderID:   taker.ID,
			Side:      taker.Side,
			Price:     taker.Price,
			OrderType: taker.Type,
			Amount:    taker.Amount,
			TIF:       taker.TIF,
		},
	}

	// 挂单撮合
	for taker.Remaining > 0 {
		opposite := taker.Side.Opposite()
		best, _, ok := e.best(opposite)
		if !ok || !cross(taker.Side, taker.Price, best) {
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
			Seq:          e.seq,
			TradeID:      e.tradeID,
			Price:        maker.Price,
			Amount:       q,
			TakerSide:    taker.Side,
			TakerOrderID: taker.ID,
			MakerOrderID: maker.ID,
		})
		taker.Remaining -= q
		if err := e.ob.Fill(maker.ID, q); err != nil {
			panic("Fill failed: " + err.Error())
		}
	}
	if taker.Remaining > 0 {
		// 成交后还有余钱就入book
		if err := e.ob.Place(taker); err != nil {
			panic("Place failed: " + err.Error())
		}

	}
	return events

}

func (e *Engine) applyCancel(cmd CancelOrder) []Event {
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

func cross(takerSide types.Side, limit uint64, opposite uint64) bool {
	if takerSide == types.Buy {
		return opposite <= limit
	}
	return opposite >= limit

}
