package matching

import (
	"encoding/json"
	"fmt"
	"my-go-exchange/internal/types"
)

type EventType string

const (
	EventOrderAccepted  EventType = "order_accepted"
	EventOrderCancelled EventType = "order_cancelled"
	EventOrderRejected  EventType = "order_rejected"
	EventTrade          EventType = "trade"
)

type OrderAccepted struct {
	Seq       uint64          `json:"seq"`
	OrderID   uint64          `json:"order_id"`
	AccountID uint64          `json:"account_id"`
	Side      types.Side      `json:"side"`
	Price     uint64          `json:"price"`
	OrderType types.OrderType `json:"order_type"`
	Amount    uint64          `json:"amount"`
	TIF       types.TIF       `json:"tif"`
}

func (e *OrderAccepted) Type() EventType { return EventOrderAccepted }

func (e *OrderAccepted) Marshal() ([]byte, error) {
	return json.Marshal(e)
}

func (e *OrderAccepted) EventEnvelope() (*EventEnvelope, error) {
	payload, err := json.Marshal(e)
	if err != nil {
		return nil, err
	}
	return &EventEnvelope{MsgType: e.Type(), Payload: payload}, nil
}

type OrderCancelled struct {
	Seq       uint64 `json:"seq"`
	OrderID   uint64 `json:"order_id"`
	Remaining uint64 `json:"remaining"`
}

func (e *OrderCancelled) Type() EventType { return EventOrderCancelled }

func (e *OrderCancelled) Marshal() ([]byte, error) {
	return json.Marshal(e)
}

func (e *OrderCancelled) EventEnvelope() (*EventEnvelope, error) {
	payload, err := json.Marshal(e)
	if err != nil {
		return nil, err
	}
	return &EventEnvelope{MsgType: e.Type(), Payload: payload}, nil
}

type OrderRejected struct {
	Seq    uint64 `json:"seq"`
	Reason string `json:"reason"`
}

func (e *OrderRejected) Type() EventType { return EventOrderRejected }

func (e *OrderRejected) Marshal() ([]byte, error) {
	return json.Marshal(e)
}

func (e *OrderRejected) EventEnvelope() (*EventEnvelope, error) {
	payload, err := json.Marshal(e)
	if err != nil {
		return nil, err
	}
	return &EventEnvelope{MsgType: e.Type(), Payload: payload}, nil
}

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

func (e *Trade) Type() EventType { return EventTrade }

func (e *Trade) Marshal() ([]byte, error) {
	return json.Marshal(e)
}

func (e *Trade) EventEnvelope() (*EventEnvelope, error) {
	payload, err := json.Marshal(e)
	if err != nil {
		return nil, err
	}
	return &EventEnvelope{MsgType: e.Type(), Payload: payload}, nil
}

// Event 是所有事件的公共接口
type Event interface {
	Type() EventType
	Marshal() ([]byte, error)
	EventEnvelope() (*EventEnvelope, error)
}

// EventEnvelope 用于多态序列化，支持混合事件流
type EventEnvelope struct {
	MsgType EventType       `json:"type"`
	Payload json.RawMessage `json:"payload"`
}

// MarshalEvent 将任意 Event 序列化为 Envelope JSON
func MarshalEvent(event Event) ([]byte, error) {
	env, err := event.EventEnvelope()
	if err != nil {
		return nil, err
	}
	return json.Marshal(env)
}

// UnmarshalEvent 从 Envelope JSON 反序列化为具体事件
func UnmarshalEvent(data []byte) (Event, error) {
	var env EventEnvelope
	if err := json.Unmarshal(data, &env); err != nil {
		return nil, fmt.Errorf("failed to unmarshal event envelope: %w", err)
	}

	switch env.MsgType {
	case EventOrderAccepted:
		var e OrderAccepted
		if err := json.Unmarshal(env.Payload, &e); err != nil {
			return nil, err
		}
		return &e, nil
	case EventOrderCancelled:
		var e OrderCancelled
		if err := json.Unmarshal(env.Payload, &e); err != nil {
			return nil, err
		}
		return &e, nil
	case EventOrderRejected:
		var e OrderRejected
		if err := json.Unmarshal(env.Payload, &e); err != nil {
			return nil, err
		}
		return &e, nil
	case EventTrade:
		var e Trade
		if err := json.Unmarshal(env.Payload, &e); err != nil {
			return nil, err
		}
		return &e, nil
	default:
		return nil, fmt.Errorf("unknown event type: %s", env.MsgType)
	}
}
