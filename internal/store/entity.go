package store

type Account struct {
	ID           int64  `json:"id" gorm:"primaryKey"`
	Username     string `json:"username" gorm:"not null;size:64;uniqueIndex"`
	PasswordHash string `json:"-" gorm:"not null"`

	Currency string `json:"currency" gorm:"not null;size:64"` // usdt
	Balance  int64  `json:"balance" gorm:"not null;default:0"`
	Frozen   int64  `json:"frozen" gorm:"not null;default:0"`
	// Equity    int64  `gorm:"equity"` // balance + unrealized PnL + frozen
	CreatedAt int64 `json:"created_at" gorm:"autoCreateTime"`
}

type Instrument struct {
	Symbol      string `json:"symbol" gorm:"primaryKey;size:32"` // BTCUSDT
	Base        string `json:"base" gorm:"not null;size:16"`     // BTC
	Quote       string `json:"quote" gorm:"not null;size:16"`    // USDT
	MaxLeverage int64  `json:"max_leverage" gorm:"not null;default:1"`
	Status      string `json:"status" gorm:"not null;size:16;index"` // trading/closed

	FeeRate int64 `json:"fee_rate" gorm:"not null;default:0"`
}

// Position 持仓情况
type Position struct {
	ID            int64  `json:"id" gorm:"primaryKey"`
	AccountID     int64  `json:"account_id" gorm:"not null;index;uniqueIndex:idx_positions_account_symbol_side,priority:1"`
	Symbol        string `json:"symbol" gorm:"not null;size:32;uniqueIndex:idx_positions_account_symbol_side,priority:2"`
	Side          string `json:"side" gorm:"not null;size:8;uniqueIndex:idx_positions_account_symbol_side,priority:3"` // long/short
	Amount        int64  `json:"amount" gorm:"not null"`                                                               // 持仓数量
	EntryPrice    int64  `json:"entry_price" gorm:"not null"`                                                          // 开仓均价
	MarkPrice     int64  `json:"mark_price" gorm:"not null"`                                                           // 标记价格
	Leverage      int64  `json:"leverage" gorm:"not null"`
	Margin        int64  `json:"margin" gorm:"not null"`
	UnrealizedPnL int64  `json:"unrealized_pnl" gorm:"not null;default:0"` // 未实现盈亏
	RealizedPnL   int64  `json:"realized_pnl" gorm:"not null;default:0"`   // 已实现盈亏
	LiqPrice      int64  `json:"liq_price" gorm:"not null"`                // 强平价格
	CreatedAt     int64  `json:"created_at" gorm:"autoCreateTime"`
	UpdatedAt     int64  `json:"updated_at" gorm:"autoUpdateTime"`
}

type Order struct {
	ID            int    `json:"id" gorm:"primaryKey"`
	AccountID     int    `json:"account_id" gorm:"not null;index;uniqueIndex:idx_orders_account_client_id,priority:1"`
	Symbol        string `json:"symbol" gorm:"not null;size:32;index"`
	Side          string `json:"side" gorm:"not null;size:8"`          // buy/sell
	PositionSide  string `json:"position_side" gorm:"not null;size:8"` // long/short
	Type          string `json:"type" gorm:"not null;size:16"`         // market/limit
	Price         int64  `json:"price" gorm:"not null"`
	Amount        int64  `json:"amount" gorm:"not null"`
	Remaining     int64  `json:"remaining" gorm:"not null"`
	Leverage      int64  `json:"leverage" gorm:"not null"`
	State         string `json:"state" gorm:"not null;size:16;index"`
	ClientOrderID string `json:"client_order_id" gorm:"not null;size:64;uniqueIndex:idx_orders_account_client_id,priority:2"` // 客户端订单ID, 用于幂等性
	CreatedAt     int64  `json:"created_at" gorm:"autoCreateTime"`
}

// Trade matching成交记录
type Trade struct {
	ID             int    `json:"id" gorm:"primaryKey"`
	Symbol         string `json:"symbol" gorm:"not null;size:32;index"`
	Price          int64  `json:"price" gorm:"not null"`
	Amount         int64  `json:"amount" gorm:"not null"`
	MakerSide      string `json:"maker_side" gorm:"not null;size:8"`
	TakerSide      string `json:"taker_side" gorm:"not null;size:8"`
	MakerOrderID   int    `json:"maker_order_id" gorm:"not null;index"`
	TakerOrderID   int    `json:"taker_order_id" gorm:"not null;index"`
	MakerAccountID int    `json:"maker_account_id" gorm:"not null;index"`
	TakerAccountID int    `json:"taker_account_id" gorm:"not null;index"`
	CreatedAt      int64  `json:"created_at" gorm:"autoCreateTime"`
}

// Fill 成交明细
type Fill struct {
	ID        int `json:"id" gorm:"primaryKey"`
	TradeID   int `json:"trade_id" gorm:"not null;uniqueIndex:idx_fills_trade_order,priority:1"`
	OrderID   int `json:"order_id" gorm:"not null;uniqueIndex:idx_fills_trade_order,priority:2"`
	AccountID int `json:"account_id" gorm:"not null;index"`

	Symbol    string `json:"symbol" gorm:"not null;size:32;index"`
	Side      string `json:"side" gorm:"not null;size:8"`
	Price     int64  `json:"price" gorm:"not null"`
	Amount    int64  `json:"amount" gorm:"not null"`
	Fee       int64  `json:"fee" gorm:"not null;default:0"`
	Role      string `json:"role" gorm:"not null;size:8"` // maker/taker
	CreatedAt int64  `json:"created_at" gorm:"autoCreateTime"`
}

// LedgerEntry 账户资金流水。
// 每一次余额或冻结资金变化，都应该有对应的流水记录。
type LedgerEntry struct {
	ID        int64 `gorm:"primaryKey;autoIncrement"`
	AccountID int64 `gorm:"not null;index"`

	Type   string `gorm:"not null;size:32"` // deposit/withdrawal/margin_freeze/realized_pnl/trading_fee 等
	Amount int64  // 正数增加，负数减少

	ReferenceType string `gorm:"size:32"` // order/trade/position/liquidation
	ReferenceID   int64

	CreatedAt int64
}
