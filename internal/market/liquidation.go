package market

import (
	"context"
	"errors"
	"fmt"
	"time"

	"my-go-exchange/internal/constant"
	"my-go-exchange/internal/trading"
)

const liquidationMarkPriceMaxAge = 2 * time.Second

// ScanLiquidations checks this symbol's positions against the current mid-price
// and submits a market order to close each triggered position. Every trigger is
// checked again while holding the symbol lock before liquidation.
func (s *LocalSymbolMarket) ScanLiquidations(ctx context.Context) error {
	if s == nil || s.store == nil || s.engine == nil {
		return nil
	}
	if err := ctx.Err(); err != nil {
		return err
	}

	s.mu.Lock()
	positions := s.store.ListPositionsBySymbol(s.symbol)
	markPrice, ok := s.markPriceLocked(time.Now())
	s.mu.Unlock()
	if !ok {
		return nil
	}

	var errs []error
	for _, position := range positions {
		if !shouldLiquidate(position, markPrice) {
			continue
		}
		if err := s.liquidate(ctx, position.AccountID, position.PositionSide); err != nil {
			errs = append(errs, fmt.Errorf(
				"liquidate account %d %s %s: %w",
				position.AccountID,
				position.Symbol,
				position.PositionSide,
				err,
			))
		}
	}
	return errors.Join(errs...)
}

func (s *LocalSymbolMarket) liquidate(
	ctx context.Context,
	accountID int64,
	positionSide constant.PositionSide,
) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	position := s.store.GetPosition(accountID, s.symbol, positionSide)
	if position == nil || position.Amount <= 0 {
		return nil
	}
	markPrice, ok := s.markPriceLocked(time.Now())
	if !ok || !shouldLiquidate(*position, markPrice) {
		return nil
	}

	s.liquidationSeq++
	_, err := s.placeLocked(ctx, PlaceReq{
		AccountID:     accountID,
		ClientOrderID: fmt.Sprintf("liquidation-%d-%s-%d", accountID, positionSide, s.liquidationSeq),
		Type:          constant.Market,
		Symbol:        s.symbol,
		OrderSide:     liquidationOrderSide(positionSide),
		PositionSide:  positionSide,
		Amount:        position.Amount,
		Leverage:      position.Leverage,
	})
	return err
}

func (s *LocalSymbolMarket) markPriceLocked(now time.Time) (int64, bool) {
	bid, ask, ok := s.lastBook.Best()
	if !ok || bid.Price <= 0 || ask.Price <= bid.Price {
		return 0, false
	}
	if s.lastBook.Timestamp > 0 {
		sourceTime := time.UnixMilli(s.lastBook.Timestamp)
		if now.Sub(sourceTime) > liquidationMarkPriceMaxAge {
			return 0, false
		}
	}
	return bid.Price + (ask.Price-bid.Price)/2, true
}

func shouldLiquidate(position trading.Position, markPrice int64) bool {
	if position.Amount <= 0 || position.LiqPrice <= 0 || markPrice <= 0 {
		return false
	}
	switch position.PositionSide {
	case constant.Long:
		return markPrice <= position.LiqPrice
	case constant.Short:
		return markPrice >= position.LiqPrice
	default:
		return false
	}
}

func liquidationOrderSide(positionSide constant.PositionSide) constant.OrderSide {
	if positionSide == constant.Long {
		return constant.Sell
	}
	return constant.Buy
}
