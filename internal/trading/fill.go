package trading

import "my-go-exchange/internal/constant"

// TradeRecord matching成交记录
type TradeRecord struct {
	ID             int64              `json:"id"`
	Symbol         string             `json:"symbol"`
	Price          int64              `json:"price"`
	Amount         int64              `json:"amount"`
	MakerSide      constant.OrderSide `json:"maker_side"`
	TakerSide      constant.OrderSide `json:"taker_side"`
	MakerOrderID   int64              `json:"maker_order_id"`
	TakerOrderID   int64              `json:"taker_order_id"`
	MakerAccountID int64              `json:"maker_account_id"`
	TakerAccountID int64              `json:"taker_account_id"`
	CreatedAt      int64              `json:"created_at"`
}

// Fill 成交明细
type Fill struct {
	ID        int64              `json:"id"`
	TradeID   int64              `json:"trade_id"`
	OrderID   int64              `json:"order_id"`
	AccountID int64              `json:"account_id"`
	Symbol    string             `json:"symbol"`
	OrderSide constant.OrderSide `json:"order_side"`
	Price     int64              `json:"price"`
	Amount    int64              `json:"amount"`
	Fee       int64              `json:"fee"`
	Role      constant.TradeRole `json:"role"` // maker/taker
	CreatedAt int64              `json:"created_at"`
}
