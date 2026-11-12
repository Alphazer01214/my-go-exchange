package constant

type PositionEffect string

const (
	EffectOpen  PositionEffect = "open"
	EffectClose PositionEffect = "close"
)

// OrderSide identifies the direction of an order.
type OrderSide string

const (
	Buy  OrderSide = "buy"
	Sell OrderSide = "sell"
)

func (s OrderSide) String() string {
	switch s {
	case Buy:
		return "buy"
	case Sell:
		return "sell"
	default:
		return "unknown"
	}
}

func (s OrderSide) Opposite() OrderSide {
	switch s {
	case Buy:
		return Sell
	case Sell:
		return Buy
	default:
		return Buy
	}
}

type PositionSide string

const (
	Long  PositionSide = "long"
	Short PositionSide = "short"
)

func (s PositionSide) String() string {
	switch s {
	case Long:
		return "long"
	case Short:
		return "short"
	default:
		return "unknown"
	}
}

type OrderType string

const (
	Limit  OrderType = "limit"
	Market OrderType = "market"
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

type TradeRole string

const (
	Maker TradeRole = "maker"
	Taker TradeRole = "taker"
)

type InstrumentStatus string

const (
	Trading InstrumentStatus = "trading"
	Closed  InstrumentStatus = "closed"
)

type LedgerEntryType string

const (
	Deposit      LedgerEntryType = "deposit"
	Withdrawal   LedgerEntryType = "withdrawal"
	MarginFreeze LedgerEntryType = "margin_freeze"
	RealizedPnL  LedgerEntryType = "realized_pnl"
	TradingFee   LedgerEntryType = "trading_fee"
)

type ReferenceType string

const (
	OrderReference       ReferenceType = "order"
	TradeReference       ReferenceType = "trade"
	PositionReference    ReferenceType = "position"
	LiquidationReference ReferenceType = "liquidation"
)

// TIF is not considered in the current phase.
// type TIF uint8
//
// const (
// 	GTC TIF = iota // good till cancel
// 	IOC            // immediate or cancel
// 	FOK            // fill or kill
// )
//
// func (t TIF) String() string {
// 	switch t {
// 	case GTC:
// 		return "GTC"
// 	case IOC:
// 		return "IOC"
// 	case FOK:
// 		return "FOK"
// 	default:
// 		return "unknown"
// 	}
// }
