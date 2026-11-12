package marketdata

type Level struct {
	Price      int64 `json:"price"`
	Amount     int64 `json:"amount"`
	OrderCount int64 `json:"order_count"`
}

type Book struct {
	Symbol       string  `json:"symbol"`
	Bids         []Level `json:"bids"`
	Asks         []Level `json:"asks"`
	LastUpdateID int64   `json:"last_update_id"`
	Timestamp    int64   `json:"timestamp"`
}

func (b *Book) Best() (bid Level, ask Level, ok bool) {
	if b == nil || len(b.Bids) == 0 || len(b.Asks) == 0 {
		return Level{}, Level{}, false
	}
	return b.Bids[0], b.Asks[0], true
}

// Quote 最优报价
type Quote struct {
	Symbol    string `json:"symbol"`
	BidPrice  int64  `json:"bid_price"`
	BidAmount int64  `json:"bid_amount"`
	AskPrice  int64  `json:"ask_price"`
	AskAmount int64  `json:"ask_amount"`
	Timestamp int64  `json:"timestamp"`
}

// Ticker 行情信息
type Ticker struct {
	Symbol     string `json:"symbol"`
	LastPrice  int64  `json:"last_price"`
	LastAmount int64  `json:"last_amount"`
	HighPrice  int64  `json:"high_price"`
	LowPrice   int64  `json:"low_price"`
	OpenPrice  int64  `json:"open_price"`
	ClosePrice int64  `json:"close_price"`
	Volume     int64  `json:"volume"`   // 成交量
	Turnover   int64  `json:"turnover"` // 成交额
	Timestamp  int64  `json:"timestamp"`
}
