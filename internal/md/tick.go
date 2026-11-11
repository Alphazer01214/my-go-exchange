package md

// Tick is a normalized external market snapshot.
// Field semantics align with ctp-crawler's tick JSON.
type Tick struct {
	Type           string   `json:"type"`
	InstrumentID   string   `json:"instrument_id"`
	Exchange       string   `json:"exchange"`
	Price          *float64 `json:"price"`
	Volume         int64    `json:"volume"`
	Bid            *float64 `json:"bid"`
	BidVolume      int64    `json:"bid_volume"`
	Ask            *float64 `json:"ask"`
	AskVolume      int64    `json:"ask_volume"`
	OpenInterest   float64  `json:"open_interest"`
	Turnover       float64  `json:"turnover"`
	Open           *float64 `json:"open"`
	High           *float64 `json:"high"`
	Low            *float64 `json:"low"`
	PreClose       *float64 `json:"pre_close"`
	PreSettlement  *float64 `json:"pre_settlement"`
	UpperLimit     *float64 `json:"upper_limit"`
	LowerLimit     *float64 `json:"lower_limit"`
	UpdateTime     string   `json:"update_time"`
	UpdateMillisec int      `json:"update_millisec"`
	TradingDay     string   `json:"trading_day"`
	ActionDay      string   `json:"action_day"`
	Timestamp      float64  `json:"timestamp"`
	Source         string   `json:"source"`
	SourceMode     string   `json:"source_mode"`
}

// Status is a bridge lifecycle event (login/subscribe/errors).
type Status struct {
	Type         string   `json:"type"`
	Event        string   `json:"event"`
	Front        string   `json:"front,omitempty"`
	Symbols      []string `json:"symbols,omitempty"`
	InstrumentID string   `json:"instrument_id,omitempty"`
	TradingDay   string   `json:"trading_day,omitempty"`
	ErrorID      int      `json:"error_id,omitempty"`
	ErrorMsg     string   `json:"error_msg,omitempty"`
	Reason       int      `json:"reason,omitempty"`
	TickCount    int64    `json:"tick_count,omitempty"`
}
