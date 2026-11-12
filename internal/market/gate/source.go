package gate

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"

	"my-go-exchange/internal/marketdata"
)

// BaseURL 是 Gate 公开现货行情 API 入口。
const BaseURL = "https://api.gateio.ws/api/v4"

// Source 通过轮询 Gate 现货盘口获取 tokenized equity 行情。
type Source struct {
	Base     string
	HTTP     *http.Client
	Symbol   string
	Interval time.Duration
}

// NewSourceFor 返回绑定交易对的 Gate 行情源。
func NewSourceFor(symbol string) *Source {
	return &Source{
		Base:   BaseURL,
		HTTP:   &http.Client{Timeout: 20 * time.Second},
		Symbol: strings.TrimSpace(symbol),
	}
}

// Name 返回行情源名称。
func (s *Source) Name() string {
	return "gate:" + strings.TrimSpace(s.Symbol)
}

// FetchOrderBook 拉取一次完整 top-N 盘口快照。
func (s *Source) FetchOrderBook(ctx context.Context) (*marketdata.Book, error) {
	symbol := strings.TrimSpace(s.Symbol)
	if symbol == "" {
		return nil, errors.New("gate: symbol is required")
	}

	endpoint, err := url.Parse(s.base() + "/spot/order_book")
	if err != nil {
		return nil, fmt.Errorf("gate order book url: %w", err)
	}
	query := endpoint.Query()
	query.Set("currency_pair", symbol)
	query.Set("limit", "20")
	query.Set("with_id", "true")
	endpoint.RawQuery = query.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("gate order book request: %w", err)
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "my-go-exchange/1.0")

	resp, err := s.client().Do(req)
	if err != nil {
		return nil, fmt.Errorf("gate order book: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("gate order book status %d", resp.StatusCode)
	}

	var frame struct {
		ID      int64      `json:"id"`
		Current int64      `json:"current"`
		Asks    [][]string `json:"asks"`
		Bids    [][]string `json:"bids"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&frame); err != nil {
		return nil, fmt.Errorf("gate order book decode: %w", err)
	}

	book := &marketdata.Book{
		Symbol:       symbol,
		Bids:         make([]marketdata.Level, 0, len(frame.Bids)),
		Asks:         make([]marketdata.Level, 0, len(frame.Asks)),
		LastUpdateID: frame.ID,
		Timestamp:    frame.Current,
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
		return nil, errors.New("gate order book: empty bid or ask side")
	}

	sort.SliceStable(book.Bids, func(i, j int) bool {
		return book.Bids[i].Price > book.Bids[j].Price
	})
	sort.SliceStable(book.Asks, func(i, j int) bool {
		return book.Asks[i].Price < book.Asks[j].Price
	})
	if book.Bids[0].Price >= book.Asks[0].Price {
		return nil, errors.New("gate order book: crossed book")
	}
	return book, nil
}

// Run 持续轮询盘口并发送快照，直到 ctx 取消。
func (s *Source) Run(ctx context.Context, out chan<- marketdata.Book) error {
	if out == nil {
		return errors.New("gate: output channel is required")
	}
	interval := s.Interval
	if interval <= 0 {
		interval = time.Second
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		book, err := s.FetchOrderBook(ctx)
		if err == nil {
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

func (s *Source) base() string {
	base := strings.TrimRight(strings.TrimSpace(s.Base), "/")
	if base == "" {
		return BaseURL
	}
	return base
}

func (s *Source) client() *http.Client {
	if s.HTTP != nil {
		return s.HTTP
	}
	return http.DefaultClient
}

func parseLevel(row []string) (marketdata.Level, bool) {
	if len(row) < 2 {
		return marketdata.Level{}, false
	}
	price, err := marketdata.DecimalToFixed(row[0])
	if err != nil || price <= 0 {
		return marketdata.Level{}, false
	}
	amount, err := marketdata.DecimalToFixed(row[1])
	if err != nil || amount <= 0 {
		return marketdata.Level{}, false
	}
	return marketdata.Level{Price: price, Amount: amount}, true
}
