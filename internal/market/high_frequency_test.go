package market_test

import (
	"context"
	"fmt"
	"my-go-exchange/internal/trading"
	"testing"
	"time"

	"my-go-exchange/internal/constant"
	"my-go-exchange/internal/market"
)

func TestHighFrequencyPlaceAndCancelFreezesMargin(t *testing.T) {
	const orderCount = 5_000
	service := newService()
	for _, accountID := range []int64{1, 2} {
		if err := service.Deposit(context.Background(), accountID, 100_000); err != nil {
			t.Fatal(err)
		}
	}

	started := time.Now()
	orders := make([]trading.Order, 0, orderCount)
	frozenByAccount := map[int64]int64{1: 0, 2: 0}

	for i := 0; i < orderCount; i++ {
		accountID := int64(1 + i%2)
		side := constant.Sell
		price := int64(2)
		if i%2 == 0 {
			side = constant.Buy
			price = 1
		}
		order, err := service.Place(context.Background(), market.PlaceReq{
			AccountID:     accountID,
			ClientOrderID: fmt.Sprintf("exchange-hf-%d", i),
			Type:          constant.Limit,
			Symbol:        "BTC/USDT",
			OrderSide:     side,
			PositionSide:  positionSideFor(side),
			Leverage:      1,
			Price:         price,
			Amount:        1,
		})
		if err != nil {
			t.Fatalf("place %d: %v", i, err)
		}
		if order.ClientOrderID != fmt.Sprintf("exchange-hf-%d", i) ||
			order.State != constant.StateInBook || order.Remaining != 1 {
			t.Fatalf("place %d order = %+v", i, order)
		}
		orders = append(orders, order)
		frozenByAccount[accountID] += price
	}

	for _, accountID := range []int64{1, 2} {
		balance := service.Balance(context.Background(), accountID)
		if balance.Frozen != frozenByAccount[accountID] {
			t.Fatalf("account %d frozen = %d, want %d", accountID, balance.Frozen, frozenByAccount[accountID])
		}
		if balance.Available+balance.Frozen != 101_000 {
			t.Fatalf("account %d balance = %+v, want total 101000", accountID, balance)
		}
	}

	for i, order := range orders {
		if i%2 != 0 {
			continue
		}
		canceled, err := service.Cancel(context.Background(), order.AccountID, order.ID)
		if err != nil {
			t.Fatalf("cancel %d: %v", order.ID, err)
		}
		if canceled.State != constant.StateCanceled || canceled.Remaining != 1 {
			t.Fatalf("cancel %d order = %+v", order.ID, canceled)
		}
		frozenByAccount[order.AccountID] -= order.Price * order.Remaining / order.Leverage
	}

	for _, accountID := range []int64{1, 2} {
		balance := service.Balance(context.Background(), accountID)
		if balance.Frozen != frozenByAccount[accountID] {
			t.Fatalf("account %d frozen after cancels = %d, want %d",
				accountID, balance.Frozen, frozenByAccount[accountID])
		}
		if balance.Available+balance.Frozen != 101_000 {
			t.Fatalf("account %d balance after cancels = %+v", accountID, balance)
		}
	}
	t.Logf("placed and canceled %d orders in %s", orderCount, time.Since(started))
}
