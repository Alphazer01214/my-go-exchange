package market

import (
	"context"
	"errors"
	"time"

	"my-go-exchange/internal/market/binance"
	"my-go-exchange/internal/market/gate"
	"my-go-exchange/internal/market/nasdaq"
)

// NewBinanceSource 返回 Binance 现货行情源，symbol 使用本地 BTC/USDT 写法。
func NewBinanceSource(symbol string) MarketSource {
	source := binance.New(binanceSymbol(symbol))
	return sourceFunc(func(ctx context.Context, out chan<- L2Book) error {
		raw := make(chan L2Book, 1)
		done := make(chan error, 1)
		go func() {
			done <- source.Run(ctx, raw)
			close(raw)
		}()

		for {
			select {
			case <-ctx.Done():
				return nil
			case err := <-done:
				return err
			case book, ok := <-raw:
				if !ok {
					return nil
				}
				book.Symbol = symbol
				select {
				case <-ctx.Done():
					return nil
				case out <- book:
				}
			}
		}
	})
}

// NewNasdaqSource 返回 Nasdaq 行情源。
func NewNasdaqSource(symbol string) MarketSource {
	return nasdaq.NewSourceFor(symbol)
}

// NewGateSource 返回 Gate tokenized equity 行情源，symbol 使用本地 NVDA/USDT 写法。
func NewGateSource(symbol string, externalSymbol string) MarketSource {
	source := gate.NewSourceFor(externalSymbol)
	return sourceFunc(func(ctx context.Context, out chan<- L2Book) error {
		raw := make(chan L2Book, 1)
		done := make(chan error, 1)
		go func() {
			done <- source.Run(ctx, raw)
			close(raw)
		}()

		for {
			select {
			case <-ctx.Done():
				return nil
			case err := <-done:
				return err
			case book, ok := <-raw:
				if !ok {
					return nil
				}
				book.Symbol = symbol
				select {
				case <-ctx.Done():
					return nil
				case out <- book:
				}
			}
		}
	})
}

// NewLocalSource 以现有 BuildBook 规则生成确定性的本地盘口。
// 使用函数适配器实现 MarketSource，不引入新的公共领域结构体。
func NewLocalSource(symbol string) MarketSource {
	config := Config{
		Mid:      100,
		Spread:   1,
		Depth:    10,
		Levels:   5,
		Interval: 1000,
	}
	return sourceFunc(func(ctx context.Context, out chan<- L2Book) error {
		if out == nil {
			return errors.New("market: output channel is required")
		}
		ticker := time.NewTicker(time.Duration(config.Interval) * time.Millisecond)
		defer ticker.Stop()

		for {
			book := BuildBook(symbol, config)
			select {
			case <-ctx.Done():
				return nil
			case out <- book:
			}

			select {
			case <-ctx.Done():
				return nil
			case <-ticker.C:
			}
		}
	})
}

type sourceFunc func(context.Context, chan<- L2Book) error

func (f sourceFunc) Run(ctx context.Context, out chan<- L2Book) error {
	return f(ctx, out)
}
