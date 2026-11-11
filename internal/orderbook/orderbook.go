package orderbook

import (
	"container/list"
	"my-go-exchange/internal/types"
)

type Level struct {
	Price      uint64
	Amount     uint64
	OrderCount uint64
}

type priceLevel struct {
	price uint64
	//amount      uint64
	orders      *list.List
	orderCount  uint64
	totalAmount uint64
}

type entry struct {
	order   *types.Order
	price   *priceLevel
	element *list.Element
}
type sideBook struct {
	less   func(a uint64, b uint64) bool
	levels []*priceLevel
	index  map[uint64]*priceLevel // price -> level
}

func (s *sideBook) front() *types.Order {
	
}

type OrderBook struct {
	asks    *sideBook // sell
	bids    *sideBook // buy
	inst    *types.Instrument
	entries map[uint64]*entry
}

func newPriceLevel(price uint64) *priceLevel {
	return &priceLevel{
		price:  price,
		orders: list.New(),
	}
}

func newOrderBook(inst *types.Instrument) *OrderBook {
	return &OrderBook{
		asks: &sideBook{
			less: func(a uint64, b uint64) bool {
				return a < b
			},
			levels: make([]*priceLevel, 0),
			index:  make(map[uint64]*priceLevel),
		},
		bids: &sideBook{
			less: func(a uint64, b uint64) bool {
				return a < b
			},
			levels: make([]*priceLevel, 0),
			index:  make(map[uint64]*priceLevel),
		},
		inst: inst,
	}
}
