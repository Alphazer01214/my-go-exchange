package constant

import "fmt"

// State 订单状态
type State string

const (
	StateCreate   State = "create"
	StatePending  State = "pending"
	StateReject   State = "reject"
	StateAccept   State = "accept" // 只是接收，但还需要 matching
	StateInBook   State = "inbook" // in orderbook
	StatePartial  State = "partial"
	StateFilled   State = "filled"
	StateCanceled State = "canceled"
	StateExpired  State = "expired"
)

func (s State) String() string {
	switch s {
	case StateCreate:
		return "create"
	case StatePending:
		return "pending"
	case StateReject:
		return "reject"
	case StateAccept:
		return "accept"
	case StateInBook:
		return "inbook"
	case StatePartial:
		return "partial"
	case StateFilled:
		return "filled"
	case StateCanceled:
		return "canceled"
	case StateExpired:
		return "expired"
	default:
		return "unknown"
	}
}

func (s State) IsTerminal() bool {
	return s == StateCanceled || s == StateExpired || s == StateFilled || s == StateReject
}

var transition = map[State]map[State]bool{
	//StateCreate: {
	//	StatePending: true,
	//	StateReject:  true,
	//},
	StatePending: {
		StateAccept: true,
		StateReject: true,
	},
	StateAccept: {
		StateInBook:   true,
		StatePartial:  true,
		StateFilled:   true,
		StateCanceled: true,
	},
	StateInBook: {
		StatePartial:  true,
		StateFilled:   true,
		StateCanceled: true,
		StateExpired:  true,
	},
	StatePartial: {
		StateFilled:   true,
		StateCanceled: true,
		StateExpired:  true,
	},
}

// IsTransitionValid Transition 注意并发
func IsTransitionValid(from State, to State) bool {
	return transition[from][to]
}

func Transition(from State, to State) (State, error) {
	if !IsTransitionValid(from, to) {
		return StateCreate, fmt.Errorf("invalid transition from %s to %s", from, to)
	}
	return to, nil
}
