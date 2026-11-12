package api

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

func (s *Server) deposit(c *gin.Context) {
	accountID, ok := authenticatedAccountID(c)
	if !ok {
		writeError(c, 401, "unauthorized", "authenticated account is missing")
		return
	}
	var req DepositRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		writeError(c, 400, "invalid_request", err.Error())
		return
	}
	if strings.TrimSpace(req.Currency) == "" || req.Amount <= 0 {
		writeError(c, 400, "invalid_request", "currency and amount are required")
		return
	}
	if err := s.market.Deposit(c.Request.Context(), accountID, req.Currency, int64(req.Amount)); err != nil {
		writeTradingError(c, err)
		return
	}
	writeData(c, 200, gin.H{"account_id": accountID, "currency": req.Currency, "amount": req.Amount})
}

func (s *Server) listBalances(c *gin.Context) {
	accountID, ok := authenticatedAccountID(c)
	if !ok {
		writeError(c, http.StatusUnauthorized, "unauthorized", "authenticated account is missing")
		return
	}

	balances := s.market.Balances(c.Request.Context(), accountID)
	views := make([]BalanceView, 0, len(balances))
	for _, balance := range balances {
		views = append(views, balanceView(balance))
	}
	writeData(c, 200, views)
}

func (s *Server) listPositions(c *gin.Context) {
	accountID, ok := authenticatedAccountID(c)
	if !ok {
		writeError(c, http.StatusUnauthorized, "unauthorized", "authenticated account is missing")
		return
	}

	positions := s.market.Positions(c.Request.Context(), accountID)
	views := make([]PositionView, 0, len(positions))
	for _, position := range positions {
		views = append(views, positionView(position))
	}
	writeData(c, 200, views)
}
