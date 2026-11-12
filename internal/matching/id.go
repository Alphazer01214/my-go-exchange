package matching

import "sync/atomic"

// IDAllocator supplies IDs shared by all matching engines in one exchange.
type IDAllocator struct {
	orderID atomic.Int64
	tradeID atomic.Int64
}

func NewIDAllocator() *IDAllocator {
	return &IDAllocator{}
}

func (a *IDAllocator) nextOrderID() int64 {
	return a.orderID.Add(1)
}

func (a *IDAllocator) nextTradeID() int64 {
	return a.tradeID.Add(1)
}
