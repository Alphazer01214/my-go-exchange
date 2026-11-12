package api

import (
	"bytes"
	"encoding/json"
	"strconv"

	"my-go-exchange/internal/constant"
)

// FixedInt is an exact int64 fixed-point value at the API seam. It accepts a
// JSON number or string and always serializes as a string so browser clients do
// not lose precision.
type FixedInt int64

func (v *FixedInt) UnmarshalJSON(data []byte) error {
	raw := bytes.TrimSpace(data)
	if bytes.Equal(raw, []byte("null")) {
		return nil
	}

	var text string
	if len(raw) > 0 && raw[0] == '"' {
		if err := json.Unmarshal(raw, &text); err != nil {
			return err
		}
	} else {
		text = string(raw)
	}
	value, err := strconv.ParseInt(text, 10, 64)
	if err != nil {
		return err
	}
	*v = FixedInt(value)
	return nil
}

func (v FixedInt) MarshalJSON() ([]byte, error) {
	return json.Marshal(strconv.FormatInt(int64(v), 10))
}

type DepositRequest struct {
	Currency string   `json:"currency"`
	Amount   FixedInt `json:"amount"`
}

type PlaceOrderRequest struct {
	ClientOrderID string                `json:"client_order_id"`
	Type          constant.OrderType    `json:"type"`
	Symbol        string                `json:"symbol"`
	OrderSide     constant.OrderSide    `json:"order_side"`
	PositionSide  constant.PositionSide `json:"position_side"`
	Price         FixedInt              `json:"price"`
	Amount        FixedInt              `json:"amount"`
	Leverage      int64                 `json:"leverage"`
}
