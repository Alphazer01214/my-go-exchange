package constant

import "errors"

// Basic parameter validation performed before an order is accepted.
var (
	ErrInvalidAccountID = errors.New("invalid account ID: must be > 0")
	ErrInvalidOrderID   = errors.New("invalid order ID: must be > 0")
	ErrInvalidSide      = errors.New("invalid order side")
	ErrInvalidOrderType = errors.New("invalid order type")
	// TIF validation is disabled until TIF semantics are implemented.
	// ErrInvalidTIF = errors.New("invalid time-in-force")
	ErrInvalidPrice        = errors.New("invalid price: must be > 0")
	ErrInvalidAmount       = errors.New("invalid amount: must be > 0")
	ErrInvalidPositionSide = errors.New("invalid position side")
	ErrInvalidLeverage     = errors.New("invalid leverage: must be > 0")
)

// Business rules checked before matching starts.
var (
	ErrMarketOrderWithPrice  = errors.New("market order must not have a price")
	ErrMarketOrderCannotRest = errors.New("market order cannot rest in book")
	ErrDuplicateOrderID      = errors.New("duplicate order ID")
	// FOK preflight is disabled until TIF semantics are implemented.
	// ErrFOKCannotFill = errors.New("FOK order cannot be fully filled")
)

// Rejected commands keep their engine-provided reason while remaining
// distinguishable from transport or storage failures.
var ErrOrderRejected = errors.New("order rejected")

// Cancellation checks.
var (
	ErrOrderNotFound       = errors.New("order not found")
	ErrOrderNotCancellable = errors.New("order is not in a cancellable state")
)

// Exchange request checks.
var (
	ErrClientOrderIDRequired = errors.New("client order ID is required")
	ErrInstrumentMismatch    = errors.New("instrument mismatch")
	ErrOrderForbidden        = errors.New("order does not belong to account")
)
