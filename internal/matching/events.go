package matching

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"

	"my-go-exchange/internal/constant"
)

type EventType string

const (
	EventOrderAccepted EventType = "order_accepted"
	EventOrderCanceled EventType = "order_canceled"
	EventOrderRejected EventType = "order_rejected"
	EventTrade         EventType = "trade"
)

// Event is the small interface produced by Engine. Sequence exposes the
// engine-wide ordering without forcing callers to type-switch first.
type Event interface {
	Kind() EventType
	Sequence() int64
}

// OrderAccepted records an order accepted by the matching engine.
type OrderAccepted struct {
	Seq           int64                 `json:"seq"`
	OrderID       int64                 `json:"order_id"`
	AccountID     int64                 `json:"account_id"`
	Price         int64                 `json:"price"`
	Amount        int64                 `json:"amount"`
	ClientOrderID string                `json:"client_order_id,omitempty"`
	Symbol        string                `json:"symbol"`
	OrderSide     constant.OrderSide    `json:"side"`
	PositionSide  constant.PositionSide `json:"position_side"`
	Type          constant.OrderType    `json:"type"`
	Leverage      int64                 `json:"leverage"`
	// TIF       constant.TIF       `json:"tif"`
}

func (e *OrderAccepted) Kind() EventType { return EventOrderAccepted }
func (e *OrderAccepted) Sequence() int64 { return e.Seq }

// OrderCanceled records cancellation of an accepted order. Remaining is the
// quantity that had not traded when cancellation took effect.
type OrderCanceled struct {
	Seq       int64  `json:"seq"`
	OrderID   int64  `json:"order_id"`
	Remaining int64  `json:"remaining"`
	Reason    string `json:"reason,omitempty"`
}

func (e *OrderCanceled) Kind() EventType { return EventOrderCanceled }
func (e *OrderCanceled) Sequence() int64 { return e.Seq }

// OrderRejected records a command that did not complete successfully.
// OrderID is zero when rejection happened before an ID was assigned.
type OrderRejected struct {
	Seq           int64  `json:"seq"`
	OrderID       int64  `json:"order_id"`
	AccountID     int64  `json:"account_id"`
	ClientOrderID string `json:"client_order_id,omitempty"`
	Reason        string `json:"reason"`
}

func (e *OrderRejected) Kind() EventType { return EventOrderRejected }
func (e *OrderRejected) Sequence() int64 { return e.Seq }

// TradeEvent records one execution between a resting maker and an incoming
// taker.
type TradeEvent struct {
	Seq            int64              `json:"seq"`
	TradeID        int64              `json:"trade_id"`
	Price          int64              `json:"price"`
	Amount         int64              `json:"amount"`
	TakerSide      constant.OrderSide `json:"taker_side"`
	TakerOrderID   int64              `json:"taker_order_id"`
	MakerOrderID   int64              `json:"maker_order_id"`
	TakerAccountID int64              `json:"taker_account_id"`
	MakerAccountID int64              `json:"maker_account_id"`
	MakerState     constant.State     `json:"maker_state"`
	MakerRemaining int64              `json:"maker_remaining"`
	TakerState     constant.State     `json:"taker_state"`
	TakerRemaining int64              `json:"taker_remaining"`
}

func (e *TradeEvent) Kind() EventType { return EventTrade }
func (e *TradeEvent) Sequence() int64 { return e.Seq }

type eventEnvelope struct {
	Type    EventType       `json:"type"`
	Payload json.RawMessage `json:"payload"`
}

// MarshalEvent serializes a supported event as a type-tagged envelope.
func MarshalEvent(event Event) ([]byte, error) {
	var payload any
	switch e := event.(type) {
	case *OrderAccepted:
		if e == nil {
			return nil, fmt.Errorf("marshal event: nil %T", e)
		}
		payload = e
	case *OrderCanceled:
		if e == nil {
			return nil, fmt.Errorf("marshal event: nil %T", e)
		}
		payload = e
	case *OrderRejected:
		if e == nil {
			return nil, fmt.Errorf("marshal event: nil %T", e)
		}
		payload = e
	case *TradeEvent:
		if e == nil {
			return nil, fmt.Errorf("marshal event: nil %T", e)
		}
		payload = e
	default:
		return nil, fmt.Errorf("marshal event: unsupported type %T", event)
	}

	raw, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("marshal event payload: %w", err)
	}
	return json.Marshal(eventEnvelope{Type: event.Kind(), Payload: raw})
}

// UnmarshalEvent decodes a type-tagged event envelope.
func UnmarshalEvent(data []byte) (Event, error) {
	var env eventEnvelope
	if err := decodeEventJSON(data, &env); err != nil {
		return nil, fmt.Errorf("decode event envelope: %w", err)
	}
	if env.Type == "" {
		return nil, fmt.Errorf("decode event envelope: missing type")
	}
	if len(env.Payload) == 0 || string(env.Payload) == "null" {
		return nil, fmt.Errorf("decode event envelope: missing payload")
	}

	var event Event
	switch env.Type {
	case EventOrderAccepted:
		event = &OrderAccepted{}
	case EventOrderCanceled:
		event = &OrderCanceled{}
	case EventOrderRejected:
		event = &OrderRejected{}
	case EventTrade:
		event = &TradeEvent{}
	default:
		return nil, fmt.Errorf("decode event envelope: unknown type %q", env.Type)
	}

	if err := decodeEventJSON(env.Payload, event); err != nil {
		return nil, fmt.Errorf("decode %s payload: %w", env.Type, err)
	}
	return event, nil
}

func decodeEventJSON(data []byte, destination any) error {
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
