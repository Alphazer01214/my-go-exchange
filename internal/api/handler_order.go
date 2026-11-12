package api

import (
	"errors"
	"strconv"

	"my-go-exchange/internal/account"
	"my-go-exchange/internal/constant"
	"my-go-exchange/internal/market"

	"github.com/gin-gonic/gin"
)

func (s *Server) placeOrder(c *gin.Context) {
	accountID, ok := authenticatedAccountID(c)
	if !ok {
		writeError(c, 401, "unauthorized", "authenticated account is missing")
		return
	}
	var req PlaceOrderRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		writeError(c, 400, "invalid_request", err.Error())
		return
	}

	order, err := s.market.Place(c.Request.Context(), market.PlaceReq{
		AccountID:     accountID,
		ClientOrderID: req.ClientOrderID,
		Type:          req.Type,
		Symbol:        req.Symbol,
		OrderSide:     req.OrderSide,
		PositionSide:  req.PositionSide,
		Price:         int64(req.Price),
		Amount:        int64(req.Amount),
		Leverage:      req.Leverage,
	})
	if err != nil {
		writeTradingError(c, err)
		return
	}
	writeData(c, 200, orderView(order))
}

func (s *Server) getOrder(c *gin.Context) {
	accountID, ok := authenticatedAccountID(c)
	if !ok {
		writeError(c, 401, "unauthorized", "authenticated account is missing")
		return
	}
	orderID, ok := orderID(c)
	if !ok {
		return
	}
	order, err := s.market.GetOrder(c.Request.Context(), accountID, orderID)
	if err != nil {
		writeTradingError(c, err)
		return
	}
	writeData(c, 200, orderView(order))
}

func (s *Server) cancelOrder(c *gin.Context) {
	accountID, ok := authenticatedAccountID(c)
	if !ok {
		writeError(c, 401, "unauthorized", "authenticated account is missing")
		return
	}
	orderID, ok := orderID(c)
	if !ok {
		return
	}
	order, err := s.market.Cancel(c.Request.Context(), accountID, orderID)
	if err != nil {
		writeTradingError(c, err)
		return
	}
	writeData(c, 200, orderView(order))
}

func orderID(c *gin.Context) (int64, bool) {
	orderID, err := strconv.ParseInt(c.Param("orderID"), 10, 64)
	if err != nil || orderID <= 0 {
		writeError(c, 400, "invalid_request", "order_id must be a positive integer")
		return 0, false
	}
	return orderID, true
}

func writeMarketError(c *gin.Context, err error) {
	if errors.Is(err, constant.ErrInstrumentMismatch) {
		writeError(c, 404, "instrument_not_found", err.Error())
		return
	}
	writeError(c, 500, "internal_error", err.Error())
}

func writeTradingError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, constant.ErrInstrumentMismatch):
		writeError(c, 404, "instrument_not_found", err.Error())
	case errors.Is(err, constant.ErrOrderNotFound):
		writeError(c, 404, "order_not_found", err.Error())
	case errors.Is(err, constant.ErrOrderForbidden):
		writeError(c, 403, "order_forbidden", err.Error())
	case errors.Is(err, account.ErrInsufficientBalance),
		errors.Is(err, account.ErrInsufficientFrozenBalance),
		errors.Is(err, constant.ErrOrderNotCancellable),
		errors.Is(err, constant.ErrDuplicateOrderID):
		writeError(c, 409, "conflict", err.Error())
	case errors.Is(err, constant.ErrOrderRejected),
		errors.Is(err, constant.ErrInvalidAccountID),
		errors.Is(err, constant.ErrInvalidOrderID),
		errors.Is(err, constant.ErrInvalidSide),
		errors.Is(err, constant.ErrInvalidOrderType),
		errors.Is(err, constant.ErrInvalidPrice),
		errors.Is(err, constant.ErrInvalidAmount),
		errors.Is(err, constant.ErrInvalidPositionSide),
		errors.Is(err, constant.ErrInvalidLeverage),
		errors.Is(err, constant.ErrMarketOrderWithPrice),
		errors.Is(err, constant.ErrMarketOrderCannotRest),
		errors.Is(err, constant.ErrClientOrderIDRequired):
		writeError(c, 400, "invalid_order", err.Error())
	default:
		writeError(c, 500, "internal_error", err.Error())
	}
}
