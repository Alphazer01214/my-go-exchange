package entity

import "my-go-exchange/internal/constant"

type Account struct {
	ID           int64  `json:"id" gorm:"primaryKey"`
	Username     string `json:"username" gorm:"uniqueIndex"`
	PasswordHash string `json:"-"`

	// Currency string `json:"currency"` // usdt
	// Balance  int64  `json:"balance"`
	// Frozen   int64  `json:"frozen"`
	// Equity    int64  `gorm:"equity"` // balance + unrealized PnL + frozen
	CreatedAt int64 `json:"created_at" gorm:"autoCreateTime"`
}

type Balance struct {
	AccountID int64  `json:"account_id" gorm:"primaryKey"`
	Currency  string `json:"currency" gorm:"primaryKey"`
	Available int64  `json:"available"`
	Frozen    int64  `json:"frozen"`
	// Equity   int64  `gorm:"equity"` // balance + unrealized PnL + frozen
	UpdatedAt int64 `json:"updated_at" gorm:"autoUpdateTime"`
}

type Instrument struct {
	Symbol      string                    `json:"symbol" gorm:"primaryKey"` // eg BTC/USDT/sh000001
	Base        string                    `json:"base"`                     // BTC
	Quote       string                    `json:"quote"`                    // USDT
	MaxLeverage int64                     `json:"max_leverage"`
	Status      constant.InstrumentStatus `json:"status" gorm:"index"` // trading/closed

	FeeRate int64 `json:"fee_rate"`
}

// Position 持仓情况
type Position struct {
	ID           int64                 `json:"id" gorm:"primaryKey"`
	AccountID    int64                 `json:"account_id" gorm:"uniqueIndex:position_key"`
	Symbol       string                `json:"symbol" gorm:"uniqueIndex:position_key"`
	PositionSide constant.PositionSide `json:"position_side" gorm:"uniqueIndex:position_key"` // long/short

	Amount        int64 `json:"amount"`      // 持仓数量
	EntryPrice    int64 `json:"entry_price"` // 开仓均价
	MarkPrice     int64 `json:"mark_price"`  // 标记价格
	Leverage      int64 `json:"leverage"`
	Margin        int64 `json:"margin"`         // 保证金
	UnrealizedPnL int64 `json:"unrealized_pnl"` // 未实现盈亏
	RealizedPnL   int64 `json:"realized_pnl"`   // 已实现盈亏
	LiqPrice      int64 `json:"liq_price"`      // 强平价格
	CreatedAt     int64 `json:"created_at" gorm:"autoCreateTime"`
	UpdatedAt     int64 `json:"updated_at" gorm:"autoUpdateTime"`
}

type Order struct {
	ID            int64                 `json:"id" gorm:"primaryKey"`
	AccountID     int64                 `json:"account_id" gorm:"uniqueIndex:order_client"`
	Symbol        string                `json:"symbol" gorm:"index"`
	OrderSide     constant.OrderSide    `json:"order_side"`    // buy/sell
	PositionSide  constant.PositionSide `json:"position_side"` // long/short
	Type          constant.OrderType    `json:"type"`          // market/limit
	Price         int64                 `json:"price"`
	Amount        int64                 `json:"amount"` // 以某价格下单的总量
	Remaining     int64                 `json:"remaining"`
	Leverage      int64                 `json:"leverage"`
	State         constant.State        `json:"state" gorm:"index"`
	ClientOrderID string                `json:"client_order_id" gorm:"uniqueIndex:order_client"` // 客户端订单ID, 用于幂等性
	Seq           int64                 `json:"seq"`
	// TIF constant.TIF `json:"tif"` // TIF is not considered in the current phase.
	CreatedAt int64 `json:"created_at" gorm:"autoCreateTime"`
}

// Trade matching成交记录
type Trade struct {
	ID             int64              `json:"id" gorm:"primaryKey"`
	Symbol         string             `json:"symbol" gorm:"index"`
	Price          int64              `json:"price"`
	Amount         int64              `json:"amount"`
	MakerSide      constant.OrderSide `json:"maker_side"`
	TakerSide      constant.OrderSide `json:"taker_side"`
	MakerOrderID   int64              `json:"maker_order_id" gorm:"index"`
	TakerOrderID   int64              `json:"taker_order_id" gorm:"index"`
	MakerAccountID int64              `json:"maker_account_id" gorm:"index"`
	TakerAccountID int64              `json:"taker_account_id" gorm:"index"`
	CreatedAt      int64              `json:"created_at" gorm:"autoCreateTime"`
}

// Fill 成交明细
type Fill struct {
	ID        int64              `json:"id" gorm:"primaryKey"`
	TradeID   int64              `json:"trade_id" gorm:"uniqueIndex:fill_trade"`
	OrderID   int64              `json:"order_id" gorm:"uniqueIndex:fill_trade"`
	AccountID int64              `json:"account_id" gorm:"index"`
	Symbol    string             `json:"symbol" gorm:"index"`
	OrderSide constant.OrderSide `json:"order_side"`
	Price     int64              `json:"price"`
	Amount    int64              `json:"amount"`
	Fee       int64              `json:"fee"`
	Role      constant.TradeRole `json:"role"` // maker/taker
	CreatedAt int64              `json:"created_at" gorm:"autoCreateTime"`
}

// LedgerEntry 账户资金流水。
// 每一次余额或冻结资金变化，都应该有对应的流水记录。
type LedgerEntry struct {
	ID            int64                    `gorm:"primaryKey"`
	AccountID     int64                    `gorm:"index"`
	Type          constant.LedgerEntryType // deposit/withdrawal/margin_freeze/realized_pnl/trading_fee 等
	Amount        int64                    // 正数增加，负数减少
	ReferenceType constant.ReferenceType   // order/trade/position/liquidation
	ReferenceID   int64                    `gorm:"index"`
	CreatedAt     int64                    `gorm:"autoCreateTime"`
}
