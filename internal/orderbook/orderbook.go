package orderbook

import (
	"container/list"
	"fmt"
	"sort"

	"my-go-exchange/internal/errs"
	"my-go-exchange/internal/types"
)

// Level is the aggregate representation of one price level.
type Level struct {
	Price      uint64
	Amount     uint64
	OrderCount uint64
}

type priceLevel struct {
	price       uint64
	orders      *list.List
	orderCount  uint64
	totalAmount uint64
}

type entry struct {
	order   *types.Order
	level   *priceLevel
	element *list.Element
}

type sideBook struct {
	// better reports whether its first argument is closer to the top of book.
	better func(a, b uint64) bool
	levels []*priceLevel
	index  map[uint64]*priceLevel
}

func newPriceLevel(price uint64) *priceLevel {
	return &priceLevel{
		price:  price,
		orders: list.New(),
	}
}

func (s *sideBook) insertionPoint(price uint64) int {
	return sort.Search(len(s.levels), func(i int) bool {
		return s.better(price, s.levels[i].price)
	})
}

func (s *sideBook) rest(order *types.Order) *entry {
	level, ok := s.index[order.Price]
	if !ok {
		level = newPriceLevel(order.Price)
		point := s.insertionPoint(order.Price)
		s.levels = append(s.levels, nil)
		copy(s.levels[point+1:], s.levels[point:])
		s.levels[point] = level
		s.index[order.Price] = level
	}

	level.orders.PushBack(order)
	level.orderCount++
	level.totalAmount += order.Remaining

	return &entry{
		order:   order,
		level:   level,
		element: level.orders.Back(),
	}
}

// remove subtracts the order's current remaining quantity and unlinks it.
func (s *sideBook) remove(entry *entry) {
	entry.level.totalAmount -= entry.order.Remaining
	s.unlink(entry)
}

// unlink removes the order without changing aggregate quantities.
func (s *sideBook) unlink(entry *entry) {
	entry.level.orders.Remove(entry.element)
	entry.level.orderCount--
	if entry.level.orderCount == 0 {
		s.deleteLevel(entry.level.price)
	}
}

func (s *sideBook) deleteLevel(price uint64) {
	point := sort.Search(len(s.levels), func(i int) bool {
		return !s.better(s.levels[i].price, price)
	})
	if point >= len(s.levels) || s.levels[point].price != price {
		return
	}
	copy(s.levels[point:], s.levels[point+1:])
	s.levels[len(s.levels)-1] = nil
	s.levels = s.levels[:len(s.levels)-1]
	delete(s.index, price)
}

func (s *sideBook) best() (price uint64, amount uint64, ok bool) {
	if len(s.levels) == 0 {
		return 0, 0, false
	}
	level := s.levels[0]
	return level.price, level.totalAmount, true
}

func (s *sideBook) front() (types.Order, bool) {
	if len(s.levels) == 0 || s.levels[0].orders.Len() == 0 {
		return types.Order{}, false
	}
	order := s.levels[0].orders.Front().Value.(*types.Order)
	return *order, true
}

func (s *sideBook) depth(n int) []Level {
	if n <= 0 || len(s.levels) == 0 {
		return nil
	}
	if n > len(s.levels) {
		n = len(s.levels)
	}

	levels := make([]Level, 0, n)
	for _, level := range s.levels[:n] {
		levels = append(levels, Level{
			Price:      level.price,
			Amount:     level.totalAmount,
			OrderCount: level.orderCount,
		})
	}
	return levels
}

func (s *sideBook) available(takerSide types.Side, limit uint64, market bool) uint64 {
	var total uint64
	for _, level := range s.levels {
		if !market {
			if takerSide == types.Buy && level.price > limit {
				break
			}
			if takerSide == types.Sell && level.price < limit {
				break
			}
		}
		total += level.totalAmount
	}
	return total
}

// OrderBook stores one instrument's bid and ask books.
type OrderBook struct {
	inst    *types.Instrument
	bids    *sideBook
	asks    *sideBook
	entries map[uint64]*entry
	orders  map[uint64]*types.Order
}

func NewOrderBook(inst *types.Instrument) *OrderBook {
	return &OrderBook{
		inst: inst,
		bids: &sideBook{
			better: func(a, b uint64) bool { return a > b },
			levels: make([]*priceLevel, 0),
			index:  make(map[uint64]*priceLevel),
		},
		asks: &sideBook{
			better: func(a, b uint64) bool { return a < b },
			levels: make([]*priceLevel, 0),
			index:  make(map[uint64]*priceLevel),
		},
		entries: make(map[uint64]*entry),
		orders:  make(map[uint64]*types.Order),
	}
}

// ValidatePlace validates values before an order is accepted or rested.
func ValidatePlace(
	orderType types.OrderType,
	price uint64,
	amount uint64,
	side types.Side,
	tif types.TIF,
) error {
	if side != types.Buy && side != types.Sell {
		return errs.ErrInvalidSide
	}
	if orderType != types.Limit && orderType != types.Market {
		return errs.ErrInvalidOrderType
	}
	// TIF is intentionally ignored until its semantics are implemented.
	// if tif != types.GTC && tif != types.IOC && tif != types.FOK {
	// 	return errs.ErrInvalidTIF
	// }
	_ = tif
	if amount == 0 {
		return errs.ErrInvalidAmount
	}
	if orderType == types.Limit && price == 0 {
		return errs.ErrInvalidPrice
	}
	if orderType == types.Market && price != 0 {
		return errs.ErrMarketOrderWithPrice
	}
	return nil
}

// Place rests a limit order in the book.
func (ob *OrderBook) Place(order *types.Order) error {
	if order == nil {
		return fmt.Errorf("%w: nil order", errs.ErrInvalidAmount)
	}
	if order.ID == 0 {
		return errs.ErrInvalidOrderID
	}
	if order.Remaining == 0 || order.Remaining > order.Amount {
		return errs.ErrInvalidAmount
	}
	if err := ValidatePlace(order.Type, order.Price, order.Amount, order.Side, order.TIF); err != nil {
		return err
	}
	if order.Type != types.Limit {
		return errs.ErrMarketOrderCannotRest
	}
	if _, exists := ob.orders[order.ID]; exists {
		return errs.ErrDuplicateOrderID
	}

	if order.Remaining == order.Amount {
		order.State = types.StateInBook
	} else {
		order.State = types.StatePartial
	}

	var entry *entry
	if order.Side == types.Buy {
		entry = ob.bids.rest(order)
	} else {
		entry = ob.asks.rest(order)
	}
	ob.entries[order.ID] = entry
	ob.orders[order.ID] = order
	return nil
}

// Cancel removes a resting order and returns its unexecuted quantity.
func (ob *OrderBook) Cancel(id uint64) (uint64, error) {
	order, known := ob.orders[id]
	if !known {
		return 0, errs.ErrOrderNotFound
	}
	if order.State.IsTerminal() {
		return 0, errs.ErrOrderNotCancellable
	}

	entry, ok := ob.entries[id]
	if !ok {
		return 0, errs.ErrOrderNotCancellable
	}

	remaining := entry.order.Remaining
	if entry.order.Side == types.Buy {
		ob.bids.remove(entry)
	} else {
		ob.asks.remove(entry)
	}
	entry.order.State = types.StateCanceled
	delete(ob.entries, id)
	return remaining, nil
}

// Fill applies an execution to a resting order.
func (ob *OrderBook) Fill(id uint64, amount uint64) error {
	entry, ok := ob.entries[id]
	if !ok {
		return errs.ErrOrderNotFound
	}
	if amount == 0 || amount > entry.order.Remaining {
		return errs.ErrInvalidAmount
	}

	order := entry.order
	order.Remaining -= amount
	entry.level.totalAmount -= amount

	if order.Remaining == 0 {
		order.State = types.StateFilled
		if order.Side == types.Buy {
			ob.bids.unlink(entry)
		} else {
			ob.asks.unlink(entry)
		}
		delete(ob.entries, id)
		return nil
	}

	order.State = types.StatePartial
	return nil
}

// Available returns the quantity executable at or better than limit. Market
// orders pass market=true and ignore limit.
func (ob *OrderBook) Available(takerSide types.Side, limit uint64, market bool) uint64 {
	if !market && limit == 0 {
		return 0
	}
	if takerSide == types.Buy {
		return ob.asks.available(takerSide, limit, market)
	}
	if takerSide == types.Sell {
		return ob.bids.available(takerSide, limit, market)
	}
	return 0
}

// Front returns a copy of the best resting order on side.
func (ob *OrderBook) Front(side types.Side) (types.Order, bool) {
	if side == types.Buy {
		return ob.bids.front()
	}
	if side == types.Sell {
		return ob.asks.front()
	}
	return types.Order{}, false
}

func (ob *OrderBook) BestBid() (price uint64, amount uint64, ok bool) {
	return ob.bids.best()
}

func (ob *OrderBook) BestAsk() (price uint64, amount uint64, ok bool) {
	return ob.asks.best()
}

func (ob *OrderBook) Bids(n int) []Level {
	return ob.bids.depth(n)
}

func (ob *OrderBook) Asks(n int) []Level {
	return ob.asks.depth(n)
}
