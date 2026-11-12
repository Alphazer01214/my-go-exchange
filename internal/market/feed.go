package market

import (
	"context"
	"errors"
	"strings"
	"time"

	"my-go-exchange/internal/market/binance"
	"my-go-exchange/internal/market/nasdaq"
)

type MarketSource interface {
	// Run 启动行情源，持续向 out 输出 L2Book，直到 ctx 取消或发生错误。
	Run(ctx context.Context, out chan<- L2Book) error
}

// startFeed equity/cypto
func (m *LocalSymbolMarket) startFeed(ctx context.Context, book chan<- L2Book) {
	if m == nil || book == nil {
		return
	}
	if m.useBinance() {
		m.binanceFeed(ctx, book)
		return
	}
	m.nasdaqFeed(ctx, book)
}

// useBinance 按本地合约写法显式路由：BASE/QUOTE 走 Binance 现货，
// 单一股票代码走 Nasdaq。行情源内部再转换各自的外部 symbol。
func (m *LocalSymbolMarket) useBinance() bool {
	if strings.Contains(m.symbol, "/") {
		return true
	}
	if m.inst == nil || m.inst.Base == "" || m.inst.Quote == "" {
		return false
	}
	compact := strings.ToUpper(m.inst.Base + m.inst.Quote)
	return strings.EqualFold(strings.TrimSpace(m.symbol), compact)
}

func (m *LocalSymbolMarket) nasdaqFeed(ctx context.Context, book chan<- L2Book) {
	src := nasdaq.NewSource()
	tick := time.NewTicker(1 * time.Second)
	defer tick.Stop()

	for {
		if err := m.nasdaqPool(ctx, src, book); err != nil && ctx.Err() == nil {
			// log.Printf("nasdaq feed error: %v", err)
		}
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
			continue

		}
	}
}

func (m *LocalSymbolMarket) nasdaqPool(ctx context.Context, src *nasdaq.Source, book chan<- L2Book) error {
	if src == nil {
		return errors.New("nasdaq: source is required")
	}
	if book == nil {
		return errors.New("nasdaq: output channel is required")
	}
	q, err := src.FetchQuote(ctx, m.symbol)
	if err != nil {
		return err
	}
	m.mu.Lock()
	// m.lastQuote = q
	m.mu.Unlock()

	b := BuildFromQuote(m.symbol, q.Bid, q.Ask, q.Last, 5, 100)
	if _, _, ok := b.Best(); !ok {
		return errors.New("nasdaq: quote cannot build a valid book")
	}
	select {
	case <-ctx.Done():
		return ctx.Err()
	case book <- b:
		return nil
	}
}

func (m *LocalSymbolMarket) binanceFeed(ctx context.Context, out chan<- L2Book) {
	src := binance.New(binanceSymbol(m.symbol))
	raw := make(chan L2Book, 1)
	done := make(chan error, 1)
	go func() {
		done <- src.Run(ctx, raw)
		close(raw)
	}()

	for {
		select {
		case <-ctx.Done():
			return
		case err := <-done:
			if err != nil && ctx.Err() == nil {
				// log.Printf("binance feed error: %v", err)
			}
			return
		case book, ok := <-raw:
			if !ok {
				return
			}
			book.Symbol = m.symbol
			select {
			case <-ctx.Done():
				return
			case out <- book:
			}
		}
	}
}

func binanceSymbol(symbol string) string {
	return strings.ToUpper(strings.NewReplacer(
		"/", "", "-", "", "_", "", " ", "",
	).Replace(strings.TrimSpace(symbol)))
}
