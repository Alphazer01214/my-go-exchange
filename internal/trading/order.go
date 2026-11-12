package trading

import "my-go-exchange/internal/constant"

type Order struct {
	ID            int64                 `json:"id"`
	AccountID     int64                 `json:"account_id"`
	Symbol        string                `json:"symbol"`
	OrderSide     constant.OrderSide    `json:"order_side"`    // buy/sell
	PositionSide  constant.PositionSide `json:"position_side"` // long/short
	Type          constant.OrderType    `json:"type"`          // market/limit
	Price         int64                 `json:"price"`
	Amount        int64                 `json:"amount"` // 以某价格下单的总量
	Remaining     int64                 `json:"remaining"`
	Leverage      int64                 `json:"leverage"`
	State         constant.State        `json:"state"`
	ClientOrderID string                `json:"client_order_id"` // 客户端订单ID, 用于幂等性
	Seq           int64                 `json:"seq"`
	// TIF constant.TIF `json:"tif"` // TIF is not considered in the current phase.
	CreatedAt int64 `json:"created_at"`
}
