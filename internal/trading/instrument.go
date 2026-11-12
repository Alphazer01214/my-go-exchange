package trading

import "my-go-exchange/internal/constant"

type Instrument struct {
	Symbol      string                    `json:"symbol"` // eg BTC/USDT/sh000001
	Base        string                    `json:"base"`   // BTC
	Quote       string                    `json:"quote"`  // USDT
	MaxLeverage int64                     `json:"max_leverage"`
	Status      constant.InstrumentStatus `json:"status"` // trading/closed

	FeeRate int64 `json:"fee_rate"`
}
