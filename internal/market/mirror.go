package market

import (
	"fmt"
	"sort"

	"my-go-exchange/internal/constant"
	"my-go-exchange/internal/matching"
)

type Mirror struct {
	engine       *matching.Engine
	accountID    int64
	bids         map[int64]int64 // price -> mirror order id
	asks         map[int64]int64 // price -> mirror order id
	lastUpdateID int64
	timestamp    int64
}

func NewMirror(engine *matching.Engine, accountID int64) *Mirror {
	return &Mirror{
		engine:    engine,
		accountID: accountID,
		bids:      make(map[int64]int64),
		asks:      make(map[int64]int64),
	}
}

// Apply 将一份完整外部盘口快照收敛成本地镜像挂单。
// 非法、交叉或过期快照整体忽略，不能产生部分镜像结果。
func (m *Mirror) Apply(book L2Book) []matching.Event {
	return m.ApplyWithResult(book).Events
}

// ApplyWithResult converges the mirror to the target book and returns both
// matching events and the aggregate order-book changes.
func (m *Mirror) ApplyWithResult(book L2Book) matching.ApplyResult {
	if m == nil || m.engine == nil || m.accountID <= 0 {
		return matching.ApplyResult{}
	}
	bids, ok := normalizeLevels(book.Bids)
	if !ok {
		return matching.ApplyResult{}
	}
	asks, ok := normalizeLevels(book.Asks)
	if !ok || !validSpread(bids, asks) {
		return matching.ApplyResult{}
	}
	if book.LastUpdateID > 0 && book.LastUpdateID < m.lastUpdateID {
		return matching.ApplyResult{}
	}
	if book.Timestamp > 0 && book.Timestamp < m.timestamp {
		return matching.ApplyResult{}
	}

	result := matching.ApplyResult{}
	keptBids := make(map[int64]bool, len(bids))
	keptAsks := make(map[int64]bool, len(asks))

	// 先撤掉失效和数量不一致的旧档，避免新旧镜像单交叉或重复。
	result.Merge(m.reconcileSide(constant.Buy, bids, keptBids))
	result.Merge(m.reconcileSide(constant.Sell, asks, keptAsks))

	// 再补齐目标盘口。顺序固定，便于测试和事件回放。
	for _, price := range sortedPrices(bids) {
		if keptBids[price] {
			continue
		}
		result.Merge(m.place(constant.Buy, L2{Price: price, Amount: bids[price]}))
	}
	for _, price := range sortedPrices(asks) {
		if keptAsks[price] {
			continue
		}
		result.Merge(m.place(constant.Sell, L2{Price: price, Amount: asks[price]}))
	}

	if book.LastUpdateID > m.lastUpdateID {
		m.lastUpdateID = book.LastUpdateID
	}
	if book.Timestamp > m.timestamp {
		m.timestamp = book.Timestamp
	}
	return result
}

func (m *Mirror) place(side constant.OrderSide, lv L2) matching.ApplyResult {
	if m == nil || m.engine == nil || lv.Price <= 0 || lv.Amount <= 0 {
		return matching.ApplyResult{}
	}
	orders := m.ordersFor(side)
	if _, exists := orders[lv.Price]; exists {
		return matching.ApplyResult{}
	}

	positionSide := constant.Long
	if side == constant.Sell {
		positionSide = constant.Short
	}
	result := m.engine.ApplyWithResult(&matching.PlaceOrder{
		AccountID:     m.accountID,
		ClientOrderID: fmt.Sprintf("mirror-%s-%d", side, lv.Price),
		Price:         lv.Price,
		Amount:        lv.Amount,
		Leverage:      1,
		OrderSide:     side,
		PositionSide:  positionSide,
		Type:          constant.Limit,
	})

	for _, event := range result.Events {
		accepted, ok := event.(*matching.OrderAccepted)
		if !ok || accepted.AccountID != m.accountID {
			continue
		}
		order := m.engine.GetOrder(accepted.OrderID)
		if order == nil || order.Remaining <= 0 || order.State.IsTerminal() {
			continue
		}
		orders[lv.Price] = accepted.OrderID
	}
	return result
}

func (m *Mirror) cancel(id int64, side constant.OrderSide, price int64) matching.ApplyResult {
	if m == nil || m.engine == nil || id <= 0 || price <= 0 {
		return matching.ApplyResult{}
	}
	orders := m.ordersFor(side)
	if orders[price] != id {
		return matching.ApplyResult{}
	}

	order := m.engine.GetOrder(id)
	if order == nil || order.AccountID != m.accountID || order.OrderSide != side || order.Price != price {
		delete(orders, price)
		return matching.ApplyResult{}
	}
	if order.State.IsTerminal() || order.Remaining <= 0 {
		delete(orders, price)
		return matching.ApplyResult{}
	}

	result := m.engine.ApplyWithResult(&matching.CancelOrder{OrderID: id})
	for _, event := range result.Events {
		if canceled, ok := event.(*matching.OrderCanceled); ok && canceled.OrderID == id {
			delete(orders, price)
		}
	}
	return result
}

func (m *Mirror) reconcileSide(
	side constant.OrderSide,
	target map[int64]int64,
	kept map[int64]bool,
) matching.ApplyResult {
	orders := m.ordersFor(side)
	result := matching.ApplyResult{}
	for _, price := range sortedPrices(orders) {
		id := orders[price]
		order := m.engine.GetOrder(id)
		if order == nil || order.AccountID != m.accountID || order.OrderSide != side ||
			order.Price != price || order.State.IsTerminal() || order.Remaining <= 0 {
			delete(orders, price)
			continue
		}

		amount, wanted := target[price]
		if wanted && order.Remaining == amount {
			kept[price] = true
			continue
		}
		result.Merge(m.cancel(id, side, price))
	}
	return result
}

func (m *Mirror) ordersFor(side constant.OrderSide) map[int64]int64 {
	if side == constant.Buy {
		return m.bids
	}
	return m.asks
}

func normalizeLevels(levels []L2) (map[int64]int64, bool) {
	if len(levels) == 0 {
		return nil, false
	}
	normalized := make(map[int64]int64, len(levels))
	for _, level := range levels {
		if level.Price <= 0 || level.Amount <= 0 {
			return nil, false
		}
		if _, exists := normalized[level.Price]; exists {
			return nil, false
		}
		normalized[level.Price] = level.Amount
	}
	return normalized, true
}

func validSpread(bids, asks map[int64]int64) bool {
	bestBid := int64(0)
	for price := range bids {
		if price > bestBid {
			bestBid = price
		}
	}
	bestAsk := int64(0)
	for price := range asks {
		if bestAsk == 0 || price < bestAsk {
			bestAsk = price
		}
	}
	return bestBid > 0 && bestAsk > 0 && bestBid < bestAsk
}

func sortedPrices(levels map[int64]int64) []int64 {
	prices := make([]int64, 0, len(levels))
	for price := range levels {
		prices = append(prices, price)
	}
	sort.Slice(prices, func(i, j int) bool { return prices[i] < prices[j] })
	return prices
}
