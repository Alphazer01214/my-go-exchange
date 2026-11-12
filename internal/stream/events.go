package stream

import (
	"encoding/json"
	"strconv"

	"my-go-exchange/internal/constant"
)

// Fixed preserves exact fixed-point values on the WebSocket wire.
type Fixed int64

func (f Fixed) MarshalJSON() ([]byte, error) {
	return json.Marshal(strconv.FormatInt(int64(f), 10))
}

// BookLevel is one aggregated visible price level.
type BookLevel struct {
	Price      int64
	Amount     int64
	OrderCount int64
}

// Trade is the public representation of one execution.
type Trade struct {
	TradeID   int64
	Price     int64
	Amount    int64
	TakerSide constant.OrderSide
}

// OrderUpdate is a private order-state transition for one account.
type OrderUpdate struct {
	AccountID     int64
	OrderID       int64
	ClientOrderID string
	Symbol        string
	OrderSide     constant.OrderSide
	PositionSide  constant.PositionSide
	Type          constant.OrderType
	Price         int64
	Amount        int64
	Remaining     int64
	Leverage      int64
	State         constant.State
	Sequence      int64
}

// Fill is a private execution report for one order.
type Fill struct {
	AccountID     int64
	OrderID       int64
	ClientOrderID string
	TradeID       int64
	Symbol        string
	OrderSide     constant.OrderSide
	PositionSide  constant.PositionSide
	Role          constant.TradeRole
	Price         int64
	Amount        int64
}

// PrivateEventKind distinguishes private event payloads in a Commit.
type PrivateEventKind string

const (
	PrivateOrderUpdate PrivateEventKind = "order.update"
	PrivateFill        PrivateEventKind = "fill"
)

// PrivateEvent is an internal union used to preserve causal order within a
// matching commit.
type PrivateEvent struct {
	Kind  PrivateEventKind
	Order *OrderUpdate
	Fill  *Fill
}

// Commit is the immutable state-change unit published by one symbol market.
type Commit struct {
	Symbol       string
	CommandSeq   int64
	BookSequence int64
	Timestamp    int64

	Bids    []BookLevel
	Asks    []BookLevel
	Trades  []Trade
	Private []PrivateEvent
}

// Empty reports whether a commit has no publishable state change.
func (c Commit) Empty() bool {
	return len(c.Bids) == 0 &&
		len(c.Asks) == 0 &&
		len(c.Trades) == 0 &&
		len(c.Private) == 0
}
