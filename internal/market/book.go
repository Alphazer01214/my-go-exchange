package market

import (
	"context"
	"math"
	"sort"
	"time"

	"my-go-exchange/internal/marketdata"
)

// 盘口相关

type L2Feed interface {
	Run(ctx context.Context, out chan<- L2Book) error
	Name() string
}

// L2 是一个盘口档位。
type L2 = marketdata.Level

// Config 盘口配置
type Config struct {
	Mid      float64 // 中间价
	Spread   float64 // 买卖价差
	Depth    float64 // 每档数量
	Interval int     // 更新间隔，单位毫秒
	Levels   int     // 盘口档数
	Step     float64 // 盘口档位间隔
}

// BookTicker 是盘口的最新成交价和买卖价。
type BookTicker struct {
	Symbol    string `json:"symbol"`    // btcusdt
	BidPrice  int64  `json:"bid_price"` // 买一价
	BidAmount int64  `json:"bid_amount"`
	AskPrice  int64  `json:"ask_price"` // 卖一价
	AskAmount int64  `json:"ask_amount"`
	LastPrice int64  `json:"last_price"` // 最新成交价
	MarkPrice int64  `json:"mark_price"` // 标记价格
	Timestamp int64  `json:"timestamp"`  // 生成快照的时间戳，单位毫秒
}

// L2Book 是一份完整 L2 盘口快照。
type L2Book = marketdata.Book

// FixedScale 是外部十进制价格、数量转换为 int64 时使用的小数位数。
const FixedScale = marketdata.FixedScale

// DecimalToFixed 将非负十进制字符串转成 1e8 定点 int64。
func DecimalToFixed(raw string) (int64, error) {
	return marketdata.DecimalToFixed(raw)
}

func fixedFromFloat(value float64) (int64, bool) {
	return marketdata.FloatToFixed(value)
}

func BuildBook(symbol string, cfg Config) L2Book {
	book := L2Book{
		Symbol:    symbol,
		Timestamp: time.Now().UnixMilli(),
	}
	if cfg.Levels <= 0 || cfg.Depth <= 0 || cfg.Mid <= 0 || cfg.Spread <= 0 {
		return book
	}

	mid, ok := fixedFromFloat(cfg.Mid)
	if !ok {
		return book
	}
	spread, ok := fixedFromFloat(cfg.Spread)
	if !ok || spread <= 0 {
		return book
	}
	amount, ok := fixedFromFloat(cfg.Depth)
	if !ok || amount <= 0 {
		return book
	}
	stepValue := cfg.Step
	if stepValue <= 0 {
		stepValue = cfg.Spread
	}
	step, ok := fixedFromFloat(stepValue)
	if !ok || step <= 0 {
		return book
	}

	halfSpread := spread / 2
	bid := mid - halfSpread
	ask := mid + (spread - halfSpread)
	book.Bids = make([]L2, 0, cfg.Levels)
	book.Asks = make([]L2, 0, cfg.Levels)
	for i := 0; i < cfg.Levels; i++ {
		if bid <= 0 {
			break
		}
		book.Bids = append(book.Bids, L2{Price: bid, Amount: amount})
		book.Asks = append(book.Asks, L2{Price: ask, Amount: amount})
		if i == cfg.Levels-1 {
			break
		}
		if bid <= step || math.MaxInt64-ask < step {
			break
		}
		bid -= step
		ask += step
	}

	sort.SliceStable(book.Bids, func(i, j int) bool { return book.Bids[i].Price > book.Bids[j].Price })
	sort.SliceStable(book.Asks, func(i, j int) bool { return book.Asks[i].Price < book.Asks[j].Price })
	return book
}

// BuildFromQuote 把只有 top-of-book 的报价扩展成确定性的模拟多档盘口。
// 可见买卖价作为最优档，价差作为默认档距，baseQuantity 为每档数量。
func BuildFromQuote(symbol string, bid float64, ask float64, last float64, levels int, baseQuantity float64) L2Book {
	if levels <= 0 || baseQuantity <= 0 {
		return L2Book{Symbol: symbol, Timestamp: time.Now().UnixMilli()}
	}

	var mid, spread float64
	if bid > 0 && ask > 0 && bid < ask {
		mid = (bid + ask) / 2
		spread = ask - bid
	} else {
		mid = last
		if mid <= 0 {
			switch {
			case bid > 0:
				mid = bid
			case ask > 0:
				mid = ask
			default:
				return L2Book{Symbol: symbol, Timestamp: time.Now().UnixMilli()}
			}
		}
		minSpread := 0.02
		if mid < 2 {
			minSpread = math.Max(mid*0.01, 1/float64(FixedScale))
		}
		spread = math.Max(mid*0.0004, minSpread)
		if spread >= mid {
			spread = mid / 2
		}
	}
	if spread <= 0 {
		return L2Book{Symbol: symbol, Timestamp: time.Now().UnixMilli()}
	}

	return BuildBook(symbol, Config{
		Mid:    mid,
		Spread: spread,
		Depth:  baseQuantity,
		Levels: levels,
		Step:   spread,
	})
}
