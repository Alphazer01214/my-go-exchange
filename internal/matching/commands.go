package matching

import (
	"encoding/json"
	"fmt"
	"my-go-exchange/internal/types"
)

// CommandType 命令类型标识
type CommandType string

const (
	CmdPlaceOrder  CommandType = "place_order"
	CmdCancelOrder CommandType = "cancel_order"
)

type PlaceOrder struct {
	AccountID uint64          `json:"account_id"`
	Price     uint64          `json:"price"`
	Amount    uint64          `json:"amount"`
	Side      types.Side      `json:"side"`
	OrderType types.OrderType `json:"order_type"`
	TIF       types.TIF       `json:"tif"`
}

func (c *PlaceOrder) Type() CommandType { return CmdPlaceOrder }

func (c *PlaceOrder) Marshal() ([]byte, error) {
	return json.Marshal(c)
}

func (c *PlaceOrder) CommandEnvelope() (*CommandEnvelope, error) {
	payload, err := json.Marshal(c)
	if err != nil {
		return nil, err
	}
	return &CommandEnvelope{MsgType: c.Type(), Payload: payload}, nil
}

type CancelOrder struct {
	OrderID uint64 `json:"order_id"`
}

func (c *CancelOrder) Type() CommandType { return CmdCancelOrder }

func (c *CancelOrder) Marshal() ([]byte, error) {
	return json.Marshal(c)
}

func (c *CancelOrder) CommandEnvelope() (*CommandEnvelope, error) {
	payload, err := json.Marshal(c)
	if err != nil {
		return nil, err
	}
	return &CommandEnvelope{MsgType: c.Type(), Payload: payload}, nil
}

// Command 是所有命令的公共接口
type Command interface {
	Type() CommandType
	Marshal() ([]byte, error)
	CommandEnvelope() (*CommandEnvelope, error)
}

// CommandEnvelope 用于多态序列化，支持混合命令流
type CommandEnvelope struct {
	MsgType CommandType     `json:"type"`
	Payload json.RawMessage `json:"payload"`
}

// MarshalCommand 将任意 Command 序列化为 Envelope JSON
func MarshalCommand(cmd Command) ([]byte, error) {
	env, err := cmd.CommandEnvelope()
	if err != nil {
		return nil, err
	}
	return json.Marshal(env)
}

// UnmarshalCommand 从 Envelope JSON 反序列化为具体命令
func UnmarshalCommand(data []byte) (Command, error) {
	var env CommandEnvelope
	if err := json.Unmarshal(data, &env); err != nil {
		return nil, fmt.Errorf("failed to unmarshal command envelope: %w", err)
	}

	switch env.MsgType {
	case CmdPlaceOrder:
		var c PlaceOrder
		if err := json.Unmarshal(env.Payload, &c); err != nil {
			return nil, err
		}
		return &c, nil
	case CmdCancelOrder:
		var c CancelOrder
		if err := json.Unmarshal(env.Payload, &c); err != nil {
			return nil, err
		}
		return &c, nil
	default:
		return nil, fmt.Errorf("unknown command type: %s", env.MsgType)
	}
}
