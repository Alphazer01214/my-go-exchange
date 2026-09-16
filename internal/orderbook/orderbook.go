package orderbook

import (
	"container/list"
	"my-go-exchange/internal/errs"
	"my-go-exchange/internal/types"
	"sort"
)

type Level struct {
	Price      uint64
	Amount     uint64 // 标的数量
	OrderCount uint64 // 订单数
}

// priceLevel 在特定价位下订单的情况
type priceLevel struct {
	price       uint64
	orders      *list.List
	orderCount  uint64 // 订单数
	totalAmount uint64

	head *entry
	tail *entry
	next *entry
}

func newPriceLevel(price uint64) *priceLevel {
	return &priceLevel{
		price:  price,
		orders: list.New(),
	}
}

// entry 用于 o1 撤单
type entry struct {
	order   *types.Order
	level   *priceLevel
	element *list.Element
}

// sideBook ask/bid side
type sideBook struct {
	less   func(a uint64, b uint64) bool
	levels []*priceLevel          // ask: 价格从低到高排序; bid: 价格从高到低排序
	index  map[uint64]*priceLevel // 价格索引
}

func (s *sideBook) front() (*types.Order, bool) {
	if len(s.levels) == 0 {
		return nil, false
	}
	lv := s.levels[0]
	front := lv.orders.Front()
	if front == nil {
		return nil, false
	}
	return front.Value.(*types.Order), true
}

// find 使用 binary search 查找价格对应的 level **下标** (由于 levels 应该是有序的)
func (s *sideBook) find(price uint64) int {
	return sort.Search(len(s.levels), func(i int) bool {
		return s.less(price, s.levels[i].price)
	})
}

// rest 挂单
func (s *sideBook) rest(order *types.Order) *entry {
	level, ok := s.index[order.Price]
	// 挡位不存在就新建一个
	if !ok {
		level = newPriceLevel(order.Price)
		i := s.find(order.Price)
		// insert, s.levels[i] = current
		s.levels = append(append(s.levels[:i], level), s.levels[i:]...)
		s.index[order.Price] = level
	}

	level.totalAmount += order.Remaining
	level.orderCount++

	return &entry{
		order:   order,
		level:   level,
		element: level.orders.PushBack(order),
	}
}

func (s *sideBook) remove(ent *entry) {
	lv := ent.level
	lv.orders.Remove(ent.element)
	lv.totalAmount -= ent.order.Remaining
	lv.orderCount--
	if lv.orderCount == 0 {
		s.deleteLevel(lv.price)
	}
}

// deleteLevel 删除价格对应的 level
func (s *sideBook) deleteLevel(price uint64) {
	i := s.find(price)
	// find 返回的是插入位置(price < levels[i] 的第一个 i)，
	// 已有元素实际在 i-1
	if i == 0 || s.levels[i-1].price != price {
		return // 不存在该档位，防御性退出
	}
	i--
	s.levels = append(s.levels[:i], s.levels[i+1:]...)
	delete(s.index, price)
}

func (s *sideBook) best() (price uint64, totalAmount uint64, ok bool) {
	if len(s.levels) == 0 {
		return
	}
	lv := s.levels[0]
	return lv.price, lv.totalAmount, true
}

func (s *sideBook) depth(n int) []Level {
	if n <= 0 || len(s.levels) == 0 {
		return nil
	}
	if n > len(s.levels) {
		n = len(s.levels)
	}
	var levels []Level
	for idx, val := range s.levels {
		if idx >= n {
			break
		}
		lv := Level{
			Price:      val.price,
			Amount:     val.totalAmount,
			OrderCount: val.orderCount,
		}
		levels = append(levels, lv)
	}
	return levels
}

type OrderBook struct {
	inst    *types.Instrument
	bids    *sideBook // 买方
	asks    *sideBook // 卖方
	lastSeq uint64
	entries map[uint64]*entry // order id -> entry
}

func (ob *OrderBook) Place(order *types.Order) error {
	// 1. 校验 Side
	if order.Side != types.Buy && order.Side != types.Sell {
		return errs.ErrInvalidSide
	}

	// 2. 校验 OrderType
	if order.Type != types.Limit && order.Type != types.Market {
		return errs.ErrInvalidOrderType
	}

	// 3. 校验 TIF
	if order.TIF != types.GTC && order.TIF != types.IOC && order.TIF != types.FOK {
		return errs.ErrInvalidTIF
	}

	// 4. 校验 Amount
	if order.Amount == 0 {
		return errs.ErrInvalidAmount
	}

	// 5. 校验 Price (Limit 必须 > 0, Market 必须 == 0)
	if order.Type == types.Limit && order.Price == 0 {
		return errs.ErrInvalidPrice
	}
	if order.Type == types.Market && order.Price != 0 {
		return errs.ErrMarketOrderWithPrice
	}

	// 7. 检查订单 ID 是否重复
	if _, exists := ob.entries[order.ID]; exists {
		return errs.ErrDuplicateOrderID
	}

	// 8. 分配 Seq，设置状态，挂单
	ob.lastSeq++
	order.Seq = ob.lastSeq
	order.State = types.StateInBook
	if order.Side == types.Buy {
		ob.entries[order.ID] = ob.bids.rest(order)
	} else {
		ob.entries[order.ID] = ob.asks.rest(order)
	}
	return nil
}

func (ob *OrderBook) Fill(id uint64, amount uint64) error {
	ent, ok := ob.entries[id]
	if !ok {
		return errs.ErrOrderNotFound
	}
	if amount == 0 || amount > ent.order.Remaining {
		return errs.ErrInvalidAmount
	}
	ent.order.Remaining -= amount
	// order 和 level 要同步
	ent.level.totalAmount -= amount
	if ent.order.Remaining == 0 {
		if ent.order.Side == types.Buy {
			ob.bids.remove(ent)
		} else {
			ob.asks.remove(ent)
		}
		delete(ob.entries, id)
	}
	return nil
}

func (ob *OrderBook) Cancel(id uint64) (*types.Order, error) {
	// 1. 检查订单是否存在
	ent, ok := ob.entries[id]
	if !ok {
		return nil, errs.ErrOrderNotFound
	}

	// 2. 检查订单是否处于可撤销状态（非终态）
	if ent.order.State.IsTerminal() {
		return nil, errs.ErrOrderNotCancellable
	}

	// 3. 从 book 中移除
	if ent.order.Side == types.Buy {
		ob.bids.remove(ent)
	} else {
		ob.asks.remove(ent)
	}
	ent.order.State = types.StateCanceled
	delete(ob.entries, id)
	return ent.order, nil
}

func (ob *OrderBook) BestBid() (price uint64, totalAmount uint64, ok bool) {

	return ob.bids.best()
}

func (ob *OrderBook) BestAsk() (price uint64, totalAmount uint64, ok bool) {

	return ob.asks.best()
}

func (ob *OrderBook) Bids(n int) []Level {
	return ob.bids.depth(n)
}

func (ob *OrderBook) Asks(n int) []Level {
	return ob.asks.depth(n)
}

func (ob *OrderBook) Front(side types.Side) (*types.Order, bool) {
	if side == types.Buy {
		return ob.bids.front()
	}
	return ob.asks.front()
}
