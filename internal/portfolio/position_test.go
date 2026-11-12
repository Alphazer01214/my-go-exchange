package portfolio

import (
	"my-go-exchange/internal/trading"
	"testing"

	"my-go-exchange/internal/constant"
	"my-go-exchange/internal/entity"
	"my-go-exchange/internal/store"
)

func testManager() *Manager {
	positionStore := store.NewStore()
	positionStore.SaveInst(&trading.Instrument{
		Symbol: "BTC/USDT",
		Base:   "BTC",
		Quote:  "USDT",
	})
	positionStore.SaveAccount(&entity.Account{ID: 1})
	return NewManager(positionStore)
}

func TestLiquidationPrice(t *testing.T) {
	tests := []struct {
		name         string
		entryPrice   int64
		margin       int64
		amount       int64
		positionSide constant.PositionSide
		want         int64
	}{
		{name: "long", entryPrice: 100_000, margin: 20_000, amount: 10, positionSide: constant.Long, want: 98_000},
		{name: "short", entryPrice: 100_000, margin: 20_000, amount: 10, positionSide: constant.Short, want: 102_000},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := liquidationPrice(tt.entryPrice, tt.margin, tt.amount, tt.positionSide)
			if err != nil {
				t.Fatalf("liquidationPrice() error = %v", err)
			}
			if got != tt.want {
				t.Fatalf("liquidationPrice() = %d, want %d", got, tt.want)
			}
		})
	}
}

func TestClosePnL(t *testing.T) {
	tests := []struct {
		name         string
		positionSide constant.PositionSide
		entryPrice   int64
		closePrice   int64
		amount       int64
		want         int64
	}{
		{name: "long profit", positionSide: constant.Long, entryPrice: 100, closePrice: 110, amount: 2, want: 20},
		{name: "long loss", positionSide: constant.Long, entryPrice: 100, closePrice: 90, amount: 2, want: -20},
		{name: "short profit", positionSide: constant.Short, entryPrice: 100, closePrice: 90, amount: 2, want: 20},
		{name: "short loss", positionSide: constant.Short, entryPrice: 100, closePrice: 110, amount: 2, want: -20},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := realizedPnL(tt.entryPrice, tt.closePrice, tt.amount, tt.positionSide)
			if err != nil {
				t.Fatalf("realizedPnL() error = %v", err)
			}
			if got != tt.want {
				t.Fatalf("realizedPnL() = %d, want %d", got, tt.want)
			}
		})
	}
}

func TestOpenResultReturnsShortSideAndFeeDelta(t *testing.T) {
	manager := testManager()

	created, err := manager.Open(
		1,
		"BTC/USDT",
		constant.Short,
		100,
		2,
		2,
		1,
	)
	if err != nil {
		t.Fatal(err)
	}

	if created.Effect != constant.EffectOpen ||
		created.Position.PositionSide != constant.Short ||
		created.RealizedPnL != -1 ||
		created.ClosedSize != 0 ||
		created.ClosedMargin != 0 {
		t.Fatalf("created result = %+v, want short open with fee delta", created)
	}

	added, err := manager.Open(
		1,
		"BTC/USDT",
		constant.Short,
		200,
		2,
		2,
		2,
	)
	if err != nil {
		t.Fatal(err)
	}

	if added.RealizedPnL != -2 {
		t.Fatalf("added result PnL = %d, want -2", added.RealizedPnL)
	}
	if added.Position.RealizedPnL != -3 {
		t.Fatalf("position PnL = %d, want -3", added.Position.RealizedPnL)
	}
	if added.Position.Amount != 4 || added.Position.Margin != 300 {
		t.Fatalf("added position = %+v, want amount 4 and margin 300", added.Position)
	}
}

func TestCloseResultReturnsMarginAndPnLDelta(t *testing.T) {
	manager := testManager()

	if _, err := manager.Open(
		1,
		"BTC/USDT",
		constant.Long,
		100,
		4,
		2,
		0,
	); err != nil {
		t.Fatal(err)
	}

	result, err := manager.Close(
		1,
		"BTC/USDT",
		constant.Long,
		110,
		1,
		1,
	)
	if err != nil {
		t.Fatal(err)
	}

	if result.Effect != constant.EffectClose ||
		result.ClosedSize != 1 ||
		result.ClosedMargin != 50 ||
		result.RealizedPnL != 9 {
		t.Fatalf("close result = %+v, want margin 50 and PnL 9", result)
	}
	if result.Position.Amount != 3 || result.Position.Margin != 150 {
		t.Fatalf("remaining position = %+v, want amount 3 and margin 150", result.Position)
	}
	if result.Position.RealizedPnL != 9 {
		t.Fatalf("position PnL = %d, want 9", result.Position.RealizedPnL)
	}
}
