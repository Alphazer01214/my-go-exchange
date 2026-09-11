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
		best, 

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
