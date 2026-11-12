package market

import "my-go-exchange/internal/constant"

type PlaceReq struct {
	AccountID     int64                 `json:"account_id"`
	ClientOrderID string                `json:"client_order_id"`
	Type          constant.OrderType    `json:"type"` // limit/market
	Symbol        string                `json:"symbol"`
	OrderSide     constant.OrderSide    `json:"order_side"`    // buy/sell
	PositionSide  constant.PositionSide `json:"position_side"` // long/short
	Price         int64                 `json:"price"`
	Amount        int64                 `json:"amount"`
	Leverage      int64                 `json:"leverage"`
}
