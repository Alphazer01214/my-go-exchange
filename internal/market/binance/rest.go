package binance

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"

	"my-go-exchange/internal/marketdata"
)

const (
	// BaseURL 是 Binance 官方只读现货行情入口。
	// data-api 与 api.binance.com 数据一致，但不受部分地区网络限制。
	BaseURL = "https://data-api.binance.vision"
)

// RESTDepthSource 保留旧名称，便于按历史调用方式接入。
type RESTDepthSource = Source

// Source 通过轮询 REST 获取 Binance 现货 L2 盘口。
type Source struct {
	// Symbol 现货交易对，例如 BTCUSDT。
	Symbol string
	// Levels 档位数；不支持的值回退到 20。
	Levels int
	// Interval 轮询间隔；小于等于 0 时使用 500ms。
	Interval time.Duration
	// Base 可覆盖 REST 地址，主要用于测试或代理。
	Base string
	// HTTP 可覆盖 HTTP 客户端。
	HTTP *http.Client
}

// New 返回带默认配置的 Binance REST 行情源。
func New(symbol string) *Source {
	return &Source{Symbol: symbol}
}

// Name 返回行情源名称。
func (s *Source) Name() string {
	return "binance-rest:" + apiSymbol(s.Symbol)
}

// FetchDepth 拉取一次完整 top-N 盘口快照。
func (s *Source) FetchDepth(ctx context.Context) (*marketdata.Book, error) {
	return s.fetchDepth(ctx, s.httpClient())
}

// Run 持续轮询盘口并发送快照，直到 ctx 取消。
// 单次网络或响应错误不会终止行情源，下一轮会自动重试。
func (s *Source) Run(ctx context.Context, out chan<- marketdata.Book) error {
	if out == nil {
		return errors.New("binance: output channel is required")
	}
	if strings.TrimSpace(s.Symbol) == "" {
		return errors.New("binance: symbol is required")
	}
	interval := s.Interval
	if interval <= 0 {
		interval = 500 * time.Millisecond
	}
	client := s.httpClient()
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		book, err := s.fetchDepth(ctx, client)
		if err != nil {
			if ctx.Err() != nil {
				return nil
			}
		} else {
			select {
			case <-ctx.Done():
				return nil
			case out <- *book:
			}
		}

		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
		}
	}
}

func (s *Source) fetchDepth(ctx context.Context, client *http.Client) (*marketdata.Book, error) {
	symbol := apiSymbol(s.Symbol)
	if symbol == "" {
		return nil, errors.New("binance: symbol is required")
	}

	base := strings.TrimRight(strings.TrimSpace(s.Base), "/")
	if base == "" {
		base = BaseURL
	}
	endpoint, err := url.Parse(base + "/api/v3/depth")
	if err != nil {
		return nil, fmt.Errorf("binance depth url: %w", err)
	}
	query := endpoint.Query()
	query.Set("symbol", symbol)
	query.Set("limit", strconv.Itoa(s.limit()))
	endpoint.RawQuery = query.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("binance depth request: %w", err)
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "my-go-exchange/1.0")

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("binance depth: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		msg := strings.TrimSpace(string(body))
		if msg == "" {
			return nil, fmt.Errorf("binance depth status %d", resp.StatusCode)
		}
		return nil, fmt.Errorf("binance depth status %d: %s", resp.StatusCode, msg)
	}

	var frame struct {
		LastUpdateID int64      `json:"lastUpdateId"`
		Bids         [][]string `json:"bids"`
		Asks         [][]string `json:"asks"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 4<<20)).Decode(&frame); err != nil {
		return nil, fmt.Errorf("binance depth decode: %w", err)
	}

	book := &marketdata.Book{
		Symbol:       strings.ToLower(symbol),
		Bids:         make([]marketdata.Level, 0, len(frame.Bids)),
		Asks:         make([]marketdata.Level, 0, len(frame.Asks)),
		LastUpdateID: frame.LastUpdateID,
		Timestamp:    time.Now().UnixMilli(),
	}
	for _, row := range frame.Bids {
		if level, ok := parseLevel(row); ok {
			book.Bids = append(book.Bids, level)
		}
	}
	for _, row := range frame.Asks {
		if level, ok := parseLevel(row); ok {
			book.Asks = append(book.Asks, level)
		}
	}
	if len(book.Bids) == 0 || len(book.Asks) == 0 {
		return nil, errors.New("binance depth: empty bid or ask side")
	}

	// Binance 正常会按序返回，这里显式排序，保证消费方无需依赖上游顺序。
	sort.SliceStable(book.Bids, func(i, j int) bool {
		return book.Bids[i].Price > book.Bids[j].Price
	})
	sort.SliceStable(book.Asks, func(i, j int) bool {
		return book.Asks[i].Price < book.Asks[j].Price
	})
	if book.Bids[0].Price >= book.Asks[0].Price {
		return nil, errors.New("binance depth: crossed book")
	}
	return book, nil
}

func (s *Source) httpClient() *http.Client {
	if s.HTTP != nil {
		return s.HTTP
	}
	return &http.Client{Timeout: 8 * time.Second}
}

func (s *Source) limit() int {
	switch s.Levels {
	case 5, 10, 20, 50, 100, 500, 1000, 5000:
		return s.Levels
	default:
		return 20
	}
}

func parseLevel(row []string) (marketdata.Level, bool) {
	if len(row) < 2 {
		return marketdata.Level{}, false
	}
	price, err := parseFixed(row[0])
	if err != nil || price <= 0 {
		return marketdata.Level{}, false
	}
	amount, err := parseFixed(row[1])
	if err != nil || amount <= 0 {
		return marketdata.Level{}, false
	}
	return marketdata.Level{Price: price, Amount: amount}, true
}

// parseFixed 将非负十进制字符串转成 1e8 定点 int64。
func parseFixed(raw string) (int64, error) {
	return marketdata.DecimalToFixed(raw)
}

// apiSymbol 将本地 BTC/USDT 或 BTC-USDT 写法转换为 Binance BTCUSDT。
func apiSymbol(symbol string) string {
	return strings.ToUpper(strings.NewReplacer("/", "", "-", "", "_", "", " ", "").Replace(strings.TrimSpace(symbol)))
}
