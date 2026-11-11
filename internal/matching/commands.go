package matching

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"

	"my-go-exchange/internal/types"
)

// CommandType identifies a command in the serialized command stream.
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
	AccountID uint64          `json:"account_id"`
	Price     uint64          `json:"price"`
	Amount    uint64          `json:"amount"`
	Side      types.Side      `json:"side"`
	Type      types.OrderType `json:"type"`
	TIF       types.TIF       `json:"tif"`
}

func (c *PlaceOrder) Kind() CommandType { return CmdPlaceOrder }

// CancelOrder requests cancellation of an order that is still resting.
type CancelOrder struct {
	OrderID uint64 `json:"order_id"`
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

// UnmarshalCommand decodes a type-tagged command envelope.
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
		AccountID *uint64          `json:"account_id"`
		Price     *uint64          `json:"price"`
		Amount    *uint64          `json:"amount"`
		Side      *types.Side      `json:"side"`
		Type      *types.OrderType `json:"type"`
		TIF       *types.TIF       `json:"tif"`
	}
	if err := decodeCommandJSON(data, &payload); err != nil {
		return nil, fmt.Errorf("decode %s payload: %w", CmdPlaceOrder, err)
	}

	required := map[string]bool{
		"account_id": payload.AccountID != nil,
		"price":      payload.Price != nil,
		"amount":     payload.Amount != nil,
		"side":       payload.Side != nil,
		"type":       payload.Type != nil,
	}
	for _, field := range []string{"account_id", "price", "amount", "side", "type"} {
		if !required[field] {
			return nil, fmt.Errorf("decode %s payload: missing field %q", CmdPlaceOrder, field)
		}
	}

	// TIF remains optional and is copied only for forward compatibility.
	var tif types.TIF
	if payload.TIF != nil {
		tif = *payload.TIF
	}

	return &PlaceOrder{
		AccountID: *payload.AccountID,
		Price:     *payload.Price,
		Amount:    *payload.Amount,
		Side:      *payload.Side,
		Type:      *payload.Type,
		TIF:       tif,
	}, nil
}

func decodeCancelOrder(data []byte) (*CancelOrder, error) {
	var payload struct {
		OrderID *uint64 `json:"order_id"`
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
