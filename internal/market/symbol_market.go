package market

import (
	"context"

	"my-go-exchange/internal/marketdata"
	"my-go-exchange/internal/trading"
)

// SymbolMarket 单品种市场接口，可由本地撮合或外部市场实现。
type SymbolMarket interface {
	Symbol() string

	Place(ctx context.Context, req PlaceReq) (trading.Order, error)
	Cancel(ctx context.Context, accountID int64, orderID int64) (trading.Order, error)
	GetOrder(ctx context.Context, accountID int64, orderID int64) (trading.Order, error)

	ApplyBook(ctx context.Context, book marketdata.Book) error
	Book(ctx context.Context) (marketdata.Book, error)
	Start(ctx context.Context) error
	Close(ctx context.Context) error
}
