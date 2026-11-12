package matching

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"

	"my-go-exchange/internal/constant"
)

type CommandType string

const (
	CmdPlaceOrder  CommandType = "place_order"
	CmdCancelOrder CommandType = "cancel_order"
)

// Command is the small interface consumed by Engine. Kind allows generic
// transports and logs to identify a command without exposing its payload.
type Command interface {
	Kind() CommandType
}

// PlaceOrder requests that the engine accept and match an order.
type PlaceOrder struct {
	AccountID     int64                 `json:"account_id"`
	ClientOrderID string                `json:"client_order_id"`
	Price         int64                 `json:"price"`
	Amount        int64                 `json:"amount"`
	Leverage      int64                 `json:"leverage"`
	OrderSide     constant.OrderSide    `json:"side"`
	PositionSide  constant.PositionSide `json:"position_side"`
	Type          constant.OrderType    `json:"type"`
	// TIF       constant.TIF       `json:"tif"`
}

func (c *PlaceOrder) Kind() CommandType { return CmdPlaceOrder }

// CancelOrder requests cancellation of an order that is still resting.
type CancelOrder struct {
	OrderID int64 `json:"order_id"`
}

func (c *CancelOrder) Kind() CommandType { return CmdCancelOrder }

type commandEnvelope struct {
	Type    CommandType     `json:"type"`
	Payload json.RawMessage `json:"payload"`
}

// MarshalCommand serializes a supported command as a type-tagged envelope.
func MarshalCommand(cmd Command) ([]byte, error) {
	var payload any
	switch c := cmd.(type) {
	case *PlaceOrder:
		if c == nil {
			return nil, fmt.Errorf("marshal command: nil %T", c)
		}
		payload = c
	case *CancelOrder:
		if c == nil {
			return nil, fmt.Errorf("marshal command: nil %T", c)
		}
		payload = c
	default:
		return nil, fmt.Errorf("marshal command: unsupported type %T", cmd)
	}

	raw, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("marshal command payload: %w", err)
	}
	return json.Marshal(commandEnvelope{Type: cmd.Kind(), Payload: raw})
}

// UnmarshalCommand decodes a supported command from a type-tagged envelope.
func UnmarshalCommand(data []byte) (Command, error) {
	var env commandEnvelope
	if err := decodeCommandJSON(data, &env); err != nil {
		return nil, fmt.Errorf("decode command envelope: %w", err)
	}
	if env.Type == "" {
		return nil, fmt.Errorf("decode command envelope: missing type")
	}
	if len(env.Payload) == 0 || string(env.Payload) == "null" {
		return nil, fmt.Errorf("decode command envelope: missing payload")
	}

	switch env.Type {
	case CmdPlaceOrder:
		return decodePlaceOrder(env.Payload)
	case CmdCancelOrder:
		return decodeCancelOrder(env.Payload)
	default:
		return nil, fmt.Errorf("decode command envelope: unknown type %q", env.Type)
	}
}

func decodePlaceOrder(data []byte) (*PlaceOrder, error) {
	var payload struct {
		AccountID     *int64                `json:"account_id"`
		ClientOrderID string                `json:"client_order_id"`
		Price         *int64                `json:"price"`
		Amount        *int64                `json:"amount"`
		Leverage      int64                 `json:"leverage"`
		OrderSide     *constant.OrderSide   `json:"side"`
		PositionSide  constant.PositionSide `json:"position_side"`
		Type          *constant.OrderType   `json:"type"`
		// TIF       constant.TIF       `json:"tif"`
	}
	if err := decodeCommandJSON(data, &payload); err != nil {
		return nil, fmt.Errorf("decode %s payload: %w", CmdPlaceOrder, err)
	}

	required := map[string]bool{
		"account_id": payload.AccountID != nil,
		"price":      payload.Price != nil,
		"amount":     payload.Amount != nil,
		"side":       payload.OrderSide != nil,
		"type":       payload.Type != nil,
	}
	for _, field := range []string{"account_id", "price", "amount", "side", "type"} {
		if !required[field] {
			return nil, fmt.Errorf("decode %s payload: missing field %q", CmdPlaceOrder, field)
		}
	}

	return &PlaceOrder{
		AccountID:     *payload.AccountID,
		ClientOrderID: payload.ClientOrderID,
		Price:         *payload.Price,
		Amount:        *payload.Amount,
		Leverage:      payload.Leverage,
		OrderSide:     *payload.OrderSide,
		PositionSide:  payload.PositionSide,
		Type:          *payload.Type,
		// TIF:       payload.TIF,
	}, nil
}

func decodeCancelOrder(data []byte) (*CancelOrder, error) {
	var payload struct {
		OrderID *int64 `json:"order_id"`
	}
	if err := decodeCommandJSON(data, &payload); err != nil {
		return nil, fmt.Errorf("decode %s payload: %w", CmdCancelOrder, err)
	}
	if payload.OrderID == nil {
		return nil, fmt.Errorf("decode %s payload: missing field %q", CmdCancelOrder, "order_id")
	}
	return &CancelOrder{OrderID: *payload.OrderID}, nil
}

func decodeCommandJSON(data []byte, destination any) error {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(destination); err != nil {
		return err
	}

	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		if err == nil {
			return fmt.Errorf("multiple JSON values")
		}
		return fmt.Errorf("trailing data: %w", err)
	}
	return nil
}
