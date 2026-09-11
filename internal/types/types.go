package types

type Side uint8

const (
	Buy Side = iota
	Sell
)

func (s Side) String() string {
	switch s {
	case Buy:
		return "buy"
	case Sell:
		return "sell"
	default:
		return "unknown"
	}
}

func (s Side) Opposite() Side {
	switch s {
	case Buy:
		return Sell
	case Sell:
		return Buy
	default:
		return Buy
	}
}

type OrderType uint8

const (
	Limit OrderType = iota
	Market
)

func (o OrderType) String() string {
	switch o {
	case Limit:
		return "limit"
	case Market:
		return "market"
	default:
		return "unknown"
	}
}

type TIF uint8

const (
	GTC TIF = iota // good till cancel
	IOC            // immediate or cancel
	FOK            // fill or kill
)

func (t TIF) String() string {
	switch t {
	case GTC:
		return "GTC"
	case IOC:
		return "IOC"
	case FOK:
		return "FOK"
	default:
		return "unknown"
	}
}

type Order struct {
	ID        uint64
	Side      Side
	Type      OrderType
	Price     uint64
	Amount    uint64 // 以某价格下单的总量
	Remaining uint64
	TIF       TIF
	Seq       uint64
	State     State
}

type Instrument struct {
	Symbol string // eg BTC/USDT/sh000001
	Base   string
	Quote  string
}
