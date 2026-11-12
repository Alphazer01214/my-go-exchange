package orderbook

import (
	"container/list"
	"fmt"
	"my-go-exchange/internal/trading"
	"sort"

	"my-go-exchange/internal/constant"
)

// Level is the aggregate representation of one price level.
type Level struct {
	Price      int64
	Amount     int64
	OrderCount int64
}

// ChangeSet contains the final state of every price level changed while change
// tracking was enabled. A level with Amount and OrderCount set to zero was
// removed.
type ChangeSet struct {
	Bids []Level
	Asks []Level
}

// Empty reports whether the change set contains no changed levels.
func (c ChangeSet) Empty() bool {
	return len(c.Bids) == 0 && len(c.Asks) == 0
}

// Merge adds all levels from other, retaining the latest value for duplicated
// prices.
func (c *ChangeSet) Merge(other ChangeSet) {
	if c == nil {
		return
	}
	c.Bids = mergeLevels(c.Bids, other.Bids, func(a, b int64) bool { return a > b })
	c.Asks = mergeLevels(c.Asks, other.Asks, func(a, b int64) bool { return a < b })
}

type changeKey struct {
	side  constant.OrderSide
	price int64
}

type priceLevel struct {
	price       int64
	orders      *list.List
	orderCount  int64
	totalAmount int64
}

type entry struct {
	order   *trading.Order
	level   *priceLevel
	element *list.Element
}

type sideBook struct {
	// better reports whether its first argument is closer to the top of book.
	better func(a, b int64) bool
	levels []*priceLevel
	index  map[int64]*priceLevel
}

func newPriceLevel(price int64) *priceLevel {
	return &priceLevel{
		price:  price,
		orders: list.New(),
	}
}

func (s *sideBook) insertionPoint(price int64) int {
	return sort.Search(len(s.levels), func(i int) bool {
		return s.better(price, s.levels[i].price)
	})
}

func (s *sideBook) rest(order *trading.Order) *entry {
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

func (s *sideBook) deleteLevel(price int64) {
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

func (s *sideBook) best() (price int64, amount int64, ok bool) {
	if len(s.levels) == 0 {
		return 0, 0, false
	}
	level := s.levels[0]
	return level.price, level.totalAmount, true
}

func (s *sideBook) front() (*trading.Order, bool) {
	if len(s.levels) == 0 || s.levels[0].orders.Len() == 0 {
		return nil, false
	}
	order := s.levels[0].orders.Front().Value.(*trading.Order)
	return order, true
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

func (s *sideBook) available(takerSide constant.OrderSide, limit int64, market bool) int64 {
	var total int64
	for _, level := range s.levels {
		if !market {
			if takerSide == constant.Buy && level.price > limit {
				break
			}
			if takerSide == constant.Sell && level.price < limit {
				break
			}
		}
		total += level.totalAmount
	}
	return total
}

// OrderBook stores one instrument's bid and ask books.
type OrderBook struct {
	inst    *trading.Instrument
	bids    *sideBook
	asks    *sideBook
	entries map[int64]*entry

	tracking bool
	changes  map[changeKey]Level
}

func NewOrderBook(inst *trading.Instrument) *OrderBook {
	return &OrderBook{
		inst: inst,
		bids: &sideBook{
			better: func(a, b int64) bool { return a > b },
			levels: make([]*priceLevel, 0),
			index:  make(map[int64]*priceLevel),
		},
		asks: &sideBook{
			better: func(a, b int64) bool { return a < b },
			levels: make([]*priceLevel, 0),
			index:  make(map[int64]*priceLevel),
		},
		entries: make(map[int64]*entry),
	}
}

// BeginTracking starts recording changed price levels. Tracking is not safe for
// concurrent mutation; callers already serialize access to an OrderBook.
func (ob *OrderBook) BeginTracking() {
	ob.tracking = true
	ob.changes = make(map[changeKey]Level)
}

// EndTracking stops recording changes and returns the final state of each
// affected level.
func (ob *OrderBook) EndTracking() ChangeSet {
	ob.tracking = false
	if len(ob.changes) == 0 {
		ob.changes = nil
		return ChangeSet{}
	}

	changes := ChangeSet{
		Bids: make([]Level, 0),
		Asks: make([]Level, 0),
	}
	for key, level := range ob.changes {
		if key.side == constant.Buy {
			changes.Bids = append(changes.Bids, level)
		} else {
			changes.Asks = append(changes.Asks, level)
		}
	}
	sort.Slice(changes.Bids, func(i, j int) bool {
		return changes.Bids[i].Price > changes.Bids[j].Price
	})
	sort.Slice(changes.Asks, func(i, j int) bool {
		return changes.Asks[i].Price < changes.Asks[j].Price
	})

	ob.changes = nil
	return changes
}

func (ob *OrderBook) record(side constant.OrderSide, level *priceLevel) {
	if !ob.tracking || level == nil {
		return
	}
	ob.changes[changeKey{side: side, price: level.price}] = Level{
		Price:      level.price,
		Amount:     level.totalAmount,
		OrderCount: level.orderCount,
	}
}

func mergeLevels(a, b []Level, better func(a, b int64) bool) []Level {
	if len(b) == 0 {
		return a
	}
	byPrice := make(map[int64]Level, len(a)+len(b))
	for _, level := range a {
		byPrice[level.Price] = level
	}
	for _, level := range b {
		byPrice[level.Price] = level
	}
	merged := make([]Level, 0, len(byPrice))
	for _, level := range byPrice {
		merged = append(merged, level)
	}
	sort.Slice(merged, func(i, j int) bool {
		return better(merged[i].Price, merged[j].Price)
	})
	return merged
}

// ValidatePlace validates values before an order is accepted or rested.
func ValidatePlace(
	orderType constant.OrderType,
	price int64,
	amount int64,
	side constant.OrderSide,
) error {
	if side != constant.Buy && side != constant.Sell {
		return constant.ErrInvalidSide
	}
	if orderType != constant.Limit && orderType != constant.Market {
		return constant.ErrInvalidOrderType
	}
	// TIF is intentionally ignored until its semantics are implemented.
	// if tif != constant.GTC && tif != constant.IOC && tif != constant.FOK {
	// 	return constant.ErrInvalidTIF
	// }
	if amount <= 0 {
		return constant.ErrInvalidAmount
	}
	if orderType == constant.Limit && price <= 0 {
		return constant.ErrInvalidPrice
	}
	if orderType == constant.Market && price != 0 {
		return constant.ErrMarketOrderWithPrice
	}
	return nil
}

// Rest adds a limit order to the active book without changing its state.
func (ob *OrderBook) Rest(order *trading.Order) error {
	if order == nil {
		return fmt.Errorf("%w: nil order", constant.ErrInvalidAmount)
	}
	if order.ID <= 0 {
		return constant.ErrInvalidOrderID
	}
	if order.Amount <= 0 || order.Remaining <= 0 || order.Remaining > order.Amount {
		return constant.ErrInvalidAmount
	}
	if err := ValidatePlace(order.Type, order.Price, order.Amount, order.OrderSide); err != nil {
		return err
	}
	if order.Type != constant.Limit {
		return constant.ErrMarketOrderCannotRest
	}
	if _, exists := ob.entries[order.ID]; exists {
		return constant.ErrDuplicateOrderID
	}

	var entry *entry
	if order.OrderSide == constant.Buy {
		entry = ob.bids.rest(order)
	} else {
		entry = ob.asks.rest(order)
	}
	ob.record(order.OrderSide, entry.level)
	ob.entries[order.ID] = entry
	return nil
}

// Remove removes a resting order from the active book and returns its
// unexecuted quantity. The caller owns the order's state transition.
func (ob *OrderBook) Remove(id int64) (int64, error) {
	entry, ok := ob.entries[id]
	if !ok {
		return 0, constant.ErrOrderNotFound
	}

	remaining := entry.order.Remaining
	if entry.order.OrderSide == constant.Buy {
		ob.bids.remove(entry)
	} else {
		ob.asks.remove(entry)
	}
	ob.record(entry.order.OrderSide, entry.level)
	delete(ob.entries, id)
	return remaining, nil
}

// Consume removes an executed amount from a resting order's level. It does not
// modify the order itself; the matching engine owns Remaining and State.
func (ob *OrderBook) Consume(id int64, amount int64) error {
	entry, ok := ob.entries[id]
	if !ok {
		return constant.ErrOrderNotFound
	}
	if amount <= 0 || amount > entry.order.Remaining {
		return constant.ErrInvalidAmount
	}

	entry.level.totalAmount -= amount

	if amount == entry.order.Remaining {
		if entry.order.OrderSide == constant.Buy {
			ob.bids.unlink(entry)
		} else {
			ob.asks.unlink(entry)
		}
		delete(ob.entries, id)
	}
	ob.record(entry.order.OrderSide, entry.level)
	return nil
}

// Available returns the quantity executable at or better than limit. Market
// orders pass market=true and ignore limit.
func (ob *OrderBook) Available(takerSide constant.OrderSide, limit int64, market bool) int64 {
	if !market && limit == 0 {
		return 0
	}
	if takerSide == constant.Buy {
		return ob.asks.available(takerSide, limit, market)
	}
	if takerSide == constant.Sell {
		return ob.bids.available(takerSide, limit, market)
	}
	return 0
}

// Front returns the best resting order on side.
func (ob *OrderBook) Front(side constant.OrderSide) (*trading.Order, bool) {
	if side == constant.Buy {
		return ob.bids.front()
	}
	if side == constant.Sell {
		return ob.asks.front()
	}
	return nil, false
}

func (ob *OrderBook) BestBid() (price int64, amount int64, ok bool) {
	return ob.bids.best()
}

func (ob *OrderBook) BestAsk() (price int64, amount int64, ok bool) {
	return ob.asks.best()
}

func (ob *OrderBook) Bids(n int) []Level {
	return ob.bids.depth(n)
}

func (ob *OrderBook) Asks(n int) []Level {
	return ob.asks.depth(n)
}
