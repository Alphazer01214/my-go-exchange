package matching

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"

	"my-go-exchange/internal/types"
)

// EventType identifies an event in the serialized event stream.
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
	Sequence() uint64
}

// OrderAccepted records an order accepted by the matching engine.
type OrderAccepted struct {
	Seq       uint64          `json:"seq"`
	OrderID   uint64          `json:"order_id"`
	AccountID uint64          `json:"account_id"`
	Price     uint64          `json:"price"`
	Amount    uint64          `json:"amount"`
	Side      types.Side      `json:"side"`
	Type      types.OrderType `json:"type"`
	TIF       types.TIF       `json:"tif"`
}

func (e *OrderAccepted) Kind() EventType  { return EventOrderAccepted }
func (e *OrderAccepted) Sequence() uint64 { return e.Seq }

// OrderCanceled records cancellation of an accepted order. Remaining is the
// quantity that had not traded when cancellation took effect.
type OrderCanceled struct {
	Seq       uint64 `json:"seq"`
	OrderID   uint64 `json:"order_id"`
	Remaining uint64 `json:"remaining"`
}

func (e *OrderCanceled) Kind() EventType  { return EventOrderCanceled }
func (e *OrderCanceled) Sequence() uint64 { return e.Seq }

// OrderRejected records a command that did not complete successfully.
// OrderID is zero when rejection happened before an ID was assigned.
type OrderRejected struct {
	Seq     uint64 `json:"seq"`
	OrderID uint64 `json:"order_id"`
	Reason  string `json:"reason"`
}

func (e *OrderRejected) Kind() EventType  { return EventOrderRejected }
func (e *OrderRejected) Sequence() uint64 { return e.Seq }

// Trade records one execution between a resting maker and an incoming taker.
type Trade struct {
	Seq            uint64     `json:"seq"`
	TradeID        uint64     `json:"trade_id"`
	Price          uint64     `json:"price"`
	Amount         uint64     `json:"amount"`
	TakerSide      types.Side `json:"taker_side"`
	TakerOrderID   uint64     `json:"taker_order_id"`
	MakerOrderID   uint64     `json:"maker_order_id"`
	TakerAccountID uint64     `json:"taker_account_id"`
	MakerAccountID uint64     `json:"maker_account_id"`
}

func (e *Trade) Kind() EventType  { return EventTrade }
func (e *Trade) Sequence() uint64 { return e.Seq }

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
	case *Trade:
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
		event = &Trade{}
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
