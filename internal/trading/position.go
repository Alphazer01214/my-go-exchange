package trading

import "my-go-exchange/internal/constant"

// Position 持仓情况
type Position struct {
	ID           int64                 `json:"id"`
	AccountID    int64                 `json:"account_id"`
	Symbol       string                `json:"symbol"`
	PositionSide constant.PositionSide `json:"position_side"` // long/short

	Amount        int64 `json:"amount"`      // 持仓数量
	EntryPrice    int64 `json:"entry_price"` // 开仓均价
	MarkPrice     int64 `json:"mark_price"`  // 标记价格
	Leverage      int64 `json:"leverage"`
	Margin        int64 `json:"margin"`         // 保证金
	UnrealizedPnL int64 `json:"unrealized_pnl"` // 未实现盈亏
	RealizedPnL   int64 `json:"realized_pnl"`   // 已实现盈亏
	LiqPrice      int64 `json:"liq_price"`      // 强平价格
	CreatedAt     int64 `json:"created_at"`
	UpdatedAt     int64 `json:"updated_at"`
}
