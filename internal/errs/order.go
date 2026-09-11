package errs

import "errors"

// ---- 基础参数校验 (Place 时最先检查) ----
var (
	ErrInvalidSide      = errors.New("invalid order side")
	ErrInvalidOrderType = errors.New("invalid order type")
	ErrInvalidTIF       = errors.New("invalid time-in-force")
	ErrInvalidPrice     = errors.New("invalid price: must be > 0")
	ErrInvalidAmount    = errors.New("invalid amount: must be > 0")
)

// ---- 业务规则校验 (Place 时第二步检查) ----
var (
	ErrMarketOrderWithPrice = errors.New("market order must not have a price")
	ErrDuplicateOrderID     = errors.New("duplicate order ID")
)

// ---- 撤单校验 (Cancel 时检查) ----
var (
	ErrOrderNotFound       = errors.New("order not found")
	ErrOrderNotCancellable = errors.New("order is not in a cancellable state")
)
