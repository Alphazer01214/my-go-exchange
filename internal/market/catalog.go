package market

import (
	"strings"

	"my-go-exchange/internal/constant"
	"my-go-exchange/internal/trading"
)

// 默认标的只负责启动示例和测试，不引入额外领域结构体。
var nasdaqSymbols = []string{
	"AAPL", "MSFT", "NVDA", "GOOGL", "AMZN",
	"META", "AVGO", "TSLA", "BRK.B", "LLY",
}

var binanceSymbols = []string{"BTC/USDT", "ETH/USDT"}

var localSymbols = []string{"BTC/USDT", "ETH/USDT"}

// Gate 目前可交易 9 个 Ondo tokenized equity，BRK.B 暂未找到对应交易对。
var tokenizedEquitySymbols = []string{
	"AAPL", "MSFT", "NVDA", "GOOGL", "AMZN",
	"META", "AVGO", "TSLA", "LLY",
}

func DefaultNasdaqInstruments() []trading.Instrument {
	instruments := make([]trading.Instrument, 0, len(nasdaqSymbols))
	for _, symbol := range nasdaqSymbols {
		instruments = append(instruments, trading.Instrument{
			Symbol:      symbol,
			Base:        symbol,
			Quote:       "USD",
			MaxLeverage: 100,
			Status:      constant.Trading,
		})
	}
	return instruments
}

func DefaultCryptoInstruments() []trading.Instrument {
	instruments := make([]trading.Instrument, 0, len(binanceSymbols))
	for _, symbol := range binanceSymbols {
		base, _, _ := strings.Cut(symbol, "/")
		instruments = append(instruments, trading.Instrument{
			Symbol:      symbol,
			Base:        base,
			Quote:       "USDT",
			MaxLeverage: 100,
			Status:      constant.Trading,
		})
	}
	return instruments
}

func DefaultLocalInstruments() []trading.Instrument {
	instruments := make([]trading.Instrument, 0, len(localSymbols))
	for _, symbol := range localSymbols {
		base, _, _ := strings.Cut(symbol, "/")
		instruments = append(instruments, trading.Instrument{
			Symbol:      symbol,
			Base:        base,
			Quote:       "USDT",
			MaxLeverage: 100,
			Status:      constant.Trading,
		})
	}
	return instruments
}

func DefaultTokenizedInstruments() []trading.Instrument {
	instruments := make([]trading.Instrument, 0, len(tokenizedEquitySymbols))
	for _, base := range tokenizedEquitySymbols {
		instruments = append(instruments, trading.Instrument{
			Symbol:      base + "/USDT",
			Base:        base,
			Quote:       "USDT",
			MaxLeverage: 100,
			Status:      constant.Trading,
		})
	}
	return instruments
}

// NewDefaultMarket 装载 Binance BTC/ETH、Nasdaq 美股和 Gate tokenized equity。
func NewDefaultMarket() (*Market, error) {
	exchange := NewMarket()
	instruments := append(DefaultCryptoInstruments(), DefaultNasdaqInstruments()...)
	instruments = append(instruments, DefaultTokenizedInstruments()...)
	for _, inst := range instruments {
		if _, err := exchange.RegisterLocal(&inst); err != nil {
			return nil, err
		}
	}

	for _, symbol := range binanceSymbols {
		if err := exchange.RegisterSource(symbol, NewBinanceSource(symbol)); err != nil {
			return nil, err
		}
	}
	for _, symbol := range nasdaqSymbols {
		if err := exchange.RegisterSource(symbol, NewNasdaqSource(symbol)); err != nil {
			return nil, err
		}
	}
	for _, base := range tokenizedEquitySymbols {
		symbol := base + "/USDT"
		if err := exchange.RegisterSource(symbol, NewGateSource(symbol, base+"ON_USDT")); err != nil {
			return nil, err
		}
	}
	return exchange, nil
}

// NewLocalMarket 装载本地 BTC/ETH 行情框架。
func NewLocalMarket() (*Market, error) {
	exchange := NewMarket()
	for _, inst := range DefaultLocalInstruments() {
		if _, err := exchange.RegisterLocal(&inst); err != nil {
			return nil, err
		}
		if err := exchange.RegisterSource(inst.Symbol, NewLocalSource(inst.Symbol)); err != nil {
			return nil, err
		}
	}
	return exchange, nil
}
