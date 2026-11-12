package api

import (
	"my-go-exchange/internal/constant"
	"my-go-exchange/internal/entity"
	"my-go-exchange/internal/marketdata"
	"my-go-exchange/internal/trading"

	"github.com/gin-gonic/gin"
)

type APIResponse struct {
	Data  any       `json:"data,omitempty"`
	Error *APIError `json:"error,omitempty"`
}

type APIError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

type MarketView struct {
	Symbol      string                    `json:"symbol"`
	Base        string                    `json:"base"`
	Quote       string                    `json:"quote"`
	MaxLeverage int64                     `json:"max_leverage"`
	Status      constant.InstrumentStatus `json:"status"`
	FeeRate     int64                     `json:"fee_rate"`
	PriceScale  int64                     `json:"price_scale"`
}

type BalanceView struct {
	AccountID int64    `json:"account_id"`
	Currency  string   `json:"currency"`
	Available FixedInt `json:"available"`
	Frozen    FixedInt `json:"frozen"`
	Total     FixedInt `json:"total"`
}

type OrderView struct {
	ID            int64                 `json:"id"`
	AccountID     int64                 `json:"account_id"`
	ClientOrderID string                `json:"client_order_id"`
	Symbol        string                `json:"symbol"`
	OrderSide     constant.OrderSide    `json:"order_side"`
	PositionSide  constant.PositionSide `json:"position_side"`
	Type          constant.OrderType    `json:"type"`
	Price         FixedInt              `json:"price"`
	Amount        FixedInt              `json:"amount"`
	Remaining     FixedInt              `json:"remaining"`
	Leverage      int64                 `json:"leverage"`
	State         constant.State        `json:"state"`
	Seq           int64                 `json:"seq"`
}

type PositionView struct {
	AccountID    int64                 `json:"account_id"`
	Symbol       string                `json:"symbol"`
	PositionSide constant.PositionSide `json:"position_side"`
	Amount       FixedInt              `json:"amount"`
	EntryPrice   FixedInt              `json:"entry_price"`
	MarkPrice    FixedInt              `json:"mark_price"`
	Leverage     int64                 `json:"leverage"`
	Margin       FixedInt              `json:"margin"`
	Unrealized   FixedInt              `json:"unrealized_pnl"`
	Realized     FixedInt              `json:"realized_pnl"`
	LiqPrice     FixedInt              `json:"liq_price"`
}

func marketView(instrument trading.Instrument) MarketView {
	return MarketView{
		Symbol:      instrument.Symbol,
		Base:        instrument.Base,
		Quote:       instrument.Quote,
		MaxLeverage: instrument.MaxLeverage,
		Status:      instrument.Status,
		FeeRate:     instrument.FeeRate,
		PriceScale:  marketdata.FixedScale,
	}
}

func balanceView(balance entity.Balance) BalanceView {
	return BalanceView{
		AccountID: balance.AccountID,
		Currency:  balance.Currency,
		Available: FixedInt(balance.Available),
		Frozen:    FixedInt(balance.Frozen),
		Total:     FixedInt(balance.Available + balance.Frozen),
	}
}

func orderView(order trading.Order) OrderView {
	return OrderView{
		ID:            order.ID,
		AccountID:     order.AccountID,
		ClientOrderID: order.ClientOrderID,
		Symbol:        order.Symbol,
		OrderSide:     order.OrderSide,
		PositionSide:  order.PositionSide,
		Type:          order.Type,
		Price:         FixedInt(order.Price),
		Amount:        FixedInt(order.Amount),
		Remaining:     FixedInt(order.Remaining),
		Leverage:      order.Leverage,
		State:         order.State,
		Seq:           order.Seq,
	}
}

func positionView(position trading.Position) PositionView {
	return PositionView{
		AccountID:    position.AccountID,
		Symbol:       position.Symbol,
		PositionSide: position.PositionSide,
		Amount:       FixedInt(position.Amount),
		EntryPrice:   FixedInt(position.EntryPrice),
		MarkPrice:    FixedInt(position.MarkPrice),
		Leverage:     position.Leverage,
		Margin:       FixedInt(position.Margin),
		Unrealized:   FixedInt(position.UnrealizedPnL),
		Realized:     FixedInt(position.RealizedPnL),
		LiqPrice:     FixedInt(position.LiqPrice),
	}
}

func writeData(c *gin.Context, status int, data any) {
	c.JSON(status, APIResponse{Data: data})
}

func writeError(c *gin.Context, status int, code string, message string) {
	c.JSON(status, APIResponse{Error: &APIError{Code: code, Message: message}})
}
