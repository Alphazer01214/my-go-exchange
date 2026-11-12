package nasdaq

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"

	"my-go-exchange/internal/marketdata"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// BaseURL 是 Nasdaq 公开行情 API 入口。
const BaseURL = "https://api.nasdaq.com"

// Quote 个股或 ETF 的最新报价。
type Quote struct {
	Symbol        string  `json:"symbol"`
	Name          string  `json:"name"`
	Last          float64 `json:"last"`
	Change        float64 `json:"change"`
	PercentChange float64 `json:"percent_change"`
	Open          float64 `json:"open"`
	High          float64 `json:"high"`
	Low           float64 `json:"low"`
	PrevClose     float64 `json:"prev_close"`
	Bid           float64 `json:"bid"`
	Ask           float64 `json:"ask"`
	Volume        int64   `json:"volume"`
	Timestamp     int64   `json:"timestamp"`
}

// Kline 是一条日线 OHLCV。
type Kline struct {
	Open      float64 `json:"open"`
	High      float64 `json:"high"`
	Low       float64 `json:"low"`
	Close     float64 `json:"close"`
	Volume    int64   `json:"volume"`
	Timestamp int64   `json:"timestamp"`
}

// Source 获取 Nasdaq 行情。
type Source struct {
	Base     string
	Http     *http.Client
	Symbol   string
	Interval time.Duration
}

// NewSource 返回使用官方地址和默认超时的行情源。
func NewSource() *Source {
	return &Source{
		Base: BaseURL,
		Http: &http.Client{Timeout: 20 * time.Second},
	}
}

// NewSourceFor 返回绑定 symbol 的行情源，满足 market.MarketSource。
func NewSourceFor(symbol string) *Source {
	source := NewSource()
	source.Symbol = strings.TrimSpace(symbol)
	return source
}

// Name 返回行情源名称。
func (s *Source) Name() string {
	return "nasdaq:" + strings.TrimSpace(s.Symbol)
}

// Run 按固定间隔获取 Quote 并输出合成 top-of-book。
func (s *Source) Run(ctx context.Context, out chan<- marketdata.Book) error {
	if out == nil {
		return errors.New("nasdaq: output channel is required")
	}
	symbol := strings.TrimSpace(s.Symbol)
	if symbol == "" {
		return errors.New("nasdaq: symbol is required")
	}
	interval := s.Interval
	if interval <= 0 {
		interval = time.Second
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		quote, err := s.FetchQuote(ctx, symbol)
		if err == nil {
			normalizeQuoteLiquidity(quote)
			book := marketdata.Book{
				Symbol: symbol,
				Bids:   make([]marketdata.Level, 0, 1),
				Asks:   make([]marketdata.Level, 0, 1),
			}
			if bid, ok := marketdata.FloatToFixed(quote.Bid); ok && bid > 0 {
				if amount, ok := marketdata.FloatToFixed(1); ok {
					book.Bids = append(book.Bids, marketdata.Level{Price: bid, Amount: amount})
				}
			}
			if ask, ok := marketdata.FloatToFixed(quote.Ask); ok && ask > 0 {
				if amount, ok := marketdata.FloatToFixed(1); ok {
					book.Asks = append(book.Asks, marketdata.Level{Price: ask, Amount: amount})
				}
			}
			book.Timestamp = quote.Timestamp
			if len(book.Bids) > 0 && len(book.Asks) > 0 {
				select {
				case <-ctx.Done():
					return nil
				case out <- book:
				}
			}
		}

		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
		}
	}
}

// FetchQuote 获取一笔最新报价。
func (s *Source) FetchQuote(ctx context.Context, symbol string) (*Quote, error) {
	symbol = strings.TrimSpace(symbol)
	if symbol == "" {
		return nil, errors.New("nasdaq: symbol is required")
	}

	endpoint := fmt.Sprintf("%s/api/quote/%s/info", s.base(), url.PathEscape(symbol))
	query := url.Values{}
	query.Set("assetclass", "stocks")
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint+"?"+query.Encode(), nil)
	if err != nil {
		return nil, fmt.Errorf("nasdaq quote request: %w", err)
	}
	s.setHeaders(req)

	resp, err := s.client().Do(req)
	if err != nil {
		return nil, fmt.Errorf("nasdaq quote: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, statusError("quote", resp)
	}

	var body struct {
		Data *struct {
			Symbol         string `json:"symbol"`
			CompanyName    string `json:"companyName"`
			LastTradeStamp string `json:"lastTradeTimestamp"`
			PrimaryData    struct {
				LastSalePrice    string `json:"lastSalePrice"`
				NetChange        string `json:"netChange"`
				PercentageChange string `json:"percentageChange"`
				BidPrice         string `json:"bidPrice"`
				AskPrice         string `json:"askPrice"`
				Volume           string `json:"volume"`
				LastTradeStamp   string `json:"lastTradeTimestamp"`
				OpenPrice        string `json:"openPrice"`
				Open             string `json:"open"`
			} `json:"primaryData"`
			SecondaryData struct {
				PreviousClose  string `json:"previousClose"`
				TodaysHigh     string `json:"todaysHigh"`
				TodaysLow      string `json:"todaysLow"`
				ShareVolume    string `json:"shareVolume"`
				LastTradeStamp string `json:"lastTradeTimestamp"`
				Open           string `json:"open"`
				TodayOpen      string `json:"todayOpen"`
				TodaysOpen     string `json:"todaysOpen"`
			} `json:"secondaryData"`
		} `json:"data"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 2<<20)).Decode(&body); err != nil {
		return nil, fmt.Errorf("nasdaq quote decode: %w", err)
	}
	if body.Data == nil {
		return nil, errors.New("nasdaq quote: response has no data")
	}

	data := body.Data
	q := &Quote{
		Symbol:        strings.ToUpper(symbol),
		Name:          data.CompanyName,
		Last:          parseNumber(data.PrimaryData.LastSalePrice),
		Change:        parseNumber(data.PrimaryData.NetChange),
		PercentChange: parsePercent(data.PrimaryData.PercentageChange),
		Open: firstNumber(
			data.PrimaryData.OpenPrice,
			data.PrimaryData.Open,
			data.SecondaryData.Open,
			data.SecondaryData.TodayOpen,
			data.SecondaryData.TodaysOpen,
		),
		High:      parseNumber(data.SecondaryData.TodaysHigh),
		Low:       parseNumber(data.SecondaryData.TodaysLow),
		PrevClose: parseNumber(data.SecondaryData.PreviousClose),
		Bid:       parseNumber(data.PrimaryData.BidPrice),
		Ask:       parseNumber(data.PrimaryData.AskPrice),
		Volume:    parseVolume(firstNonEmpty(data.PrimaryData.Volume, data.SecondaryData.ShareVolume)),
		Timestamp: parseTimestamp(firstNonEmpty(
			data.PrimaryData.LastTradeStamp,
			data.SecondaryData.LastTradeStamp,
			data.LastTradeStamp,
		)),
	}

	// quote/info 的 secondaryData 在部分市场状态下为空；缺失的 OHLC 和昨收
	// 尽量从同一标的的日线补齐。补齐失败不影响主报价返回。
	if q.Open <= 0 || q.High <= 0 || q.Low <= 0 || q.PrevClose <= 0 {
		enrichCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		klines, chartErr := s.FetchKline(enrichCtx, symbol, 5)
		cancel()
		if chartErr == nil && len(klines) > 0 {
			latest := klines[0]
			for _, kline := range klines[1:] {
				if kline.Timestamp > latest.Timestamp {
					latest = kline
				}
			}
			if q.Open <= 0 {
				q.Open = latest.Open
			}
			if q.High <= 0 {
				q.High = latest.High
			}
			if q.Low <= 0 {
				q.Low = latest.Low
			}
			if q.Last <= 0 {
				q.Last = latest.Close
			}

			previousFound := false
			var previous Kline
			for _, kline := range klines {
				if kline.Timestamp < latest.Timestamp && (!previousFound || kline.Timestamp > previous.Timestamp) {
					previous = kline
					previousFound = true
				}
			}
			if q.PrevClose <= 0 && previousFound {
				q.PrevClose = previous.Close
			}
		}
	}

	normalizeQuoteLiquidity(q)
	if q.PrevClose > 0 && q.Change == 0 && q.Last > 0 {
		q.Change = q.Last - q.PrevClose
		q.PercentChange = q.Change / q.PrevClose * 100
	}
	if q.Last <= 0 && q.Bid <= 0 && q.Ask <= 0 {
		return nil, errors.New("nasdaq quote: response has no usable price")
	}
	return q, nil
}

// normalizeQuoteLiquidity 处理盘前盘后和停牌状态下的缺失、倒挂盘价。
// Nasdaq 的 quote/info 不保证始终提供双边报价，但镜像引擎需要有效价差。
func normalizeQuoteLiquidity(q *Quote) {
	if q == nil {
		return
	}
	if q.Bid > 0 && q.Ask > 0 && q.Bid < q.Ask {
		return
	}

	mid := q.Last
	if mid <= 0 {
		mid = math.Max(q.Bid, q.Ask)
	}
	if mid <= 0 {
		return
	}

	spread := syntheticSpread(mid)
	switch {
	case q.Bid <= 0 && q.Ask <= 0:
		q.Bid = mid - spread/2
		q.Ask = mid + spread/2
	case q.Bid <= 0:
		if q.Ask < spread*2 {
			spread = q.Ask / 2
		}
		q.Bid = q.Ask - spread
	case q.Ask <= 0:
		q.Ask = q.Bid + spread
	default:
		// 双边倒挂时不信任盘价，退回最近成交价生成有效价差。
		q.Bid = mid - spread/2
		q.Ask = mid + spread/2
	}
	if q.Bid < 0 {
		q.Bid = 0
	}
}

func syntheticSpread(price float64) float64 {
	minSpread := 0.02
	if price < 2 {
		minSpread = math.Max(price*0.01, 1e-8)
	}
	spread := math.Max(price*0.0004, minSpread)
	if spread >= price {
		spread = price / 2
	}
	return spread
}

// FetchKline 获取最近 days 个自然日内的日线。
func (s *Source) FetchKline(ctx context.Context, symbol string, days int) ([]Kline, error) {
	symbol = strings.TrimSpace(symbol)
	if symbol == "" {
		return nil, errors.New("nasdaq: symbol is required")
	}
	if days <= 0 {
		days = 90
	}

	to := time.Now().UTC()
	from := to.AddDate(0, 0, -days)
	query := url.Values{}
	query.Set("assetclass", "stocks")
	query.Set("fromdate", from.Format("2006-01-02"))
	query.Set("todate", to.Format("2006-01-02"))
	endpoint := fmt.Sprintf("%s/api/quote/%s/chart?%s",
		s.base(), url.PathEscape(symbol), query.Encode())

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("nasdaq chart request: %w", err)
	}
	s.setHeaders(req)

	resp, err := s.client().Do(req)
	if err != nil {
		return nil, fmt.Errorf("nasdaq chart: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, statusError("chart", resp)
	}

	var body struct {
		Data *struct {
			Chart []struct {
				X int64 `json:"x"`
				Z struct {
					Open     string `json:"open"`
					High     string `json:"high"`
					Low      string `json:"low"`
					Close    string `json:"close"`
					Volume   string `json:"volume"`
					DateTime string `json:"dateTime"`
				} `json:"z"`
			} `json:"chart"`
		} `json:"data"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 8<<20)).Decode(&body); err != nil {
		return nil, fmt.Errorf("nasdaq chart decode: %w", err)
	}
	if body.Data == nil {
		return nil, errors.New("nasdaq chart: response has no data")
	}

	klines := make([]Kline, 0, len(body.Data.Chart))
	for _, row := range body.Data.Chart {
		z := row.Z
		kline := Kline{
			Open:      parseNumber(z.Open),
			High:      parseNumber(z.High),
			Low:       parseNumber(z.Low),
			Close:     parseNumber(z.Close),
			Volume:    parseVolume(z.Volume),
			Timestamp: chartTimestamp(row.X, z.DateTime),
		}
		if kline.Close <= 0 || kline.Timestamp <= 0 {
			continue
		}
		klines = append(klines, kline)
	}
	if len(klines) == 0 {
		return nil, fmt.Errorf("nasdaq chart: empty data for %s", symbol)
	}
	return klines, nil
}

// FetchKlines 是 FetchKline 的复数别名，兼容旧调用方。
func (s *Source) FetchKlines(ctx context.Context, symbol string, days int) ([]Kline, error) {
	return s.FetchKline(ctx, symbol, days)
}

func (s *Source) base() string {
	base := strings.TrimRight(strings.TrimSpace(s.Base), "/")
	if base == "" {
		return BaseURL
	}
	return base
}

func (s *Source) client() *http.Client {
	if s.Http != nil {
		return s.Http
	}
	return &http.Client{Timeout: 20 * time.Second}
}

func (s *Source) setHeaders(req *http.Request) {
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Accept-Language", "en-US,en;q=0.9")
	req.Header.Set("User-Agent",
		"Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 "+
			"(KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36")
}

func statusError(operation string, resp *http.Response) error {
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
	msg := strings.TrimSpace(string(body))
	if msg == "" {
		return fmt.Errorf("nasdaq %s status %d", operation, resp.StatusCode)
	}
	return fmt.Errorf("nasdaq %s status %d: %s", operation, resp.StatusCode, msg)
}

func parseNumber(raw string) float64 {
	s := strings.TrimSpace(raw)
	if isMissingNumber(s) {
		return 0
	}

	negative := strings.HasPrefix(s, "(") && strings.HasSuffix(s, ")")
	if negative {
		s = strings.TrimSpace(s[1 : len(s)-1])
	}
	s = strings.TrimPrefix(s, "$")
	s = strings.TrimSpace(s)
	s = strings.TrimSuffix(s, "%")
	s = strings.ReplaceAll(s, ",", "")
	s = strings.TrimPrefix(s, "+")
	if s == "" || strings.ContainsAny(s, "()$,% \t\r\n") {
		return 0
	}
	if negative {
		if strings.HasPrefix(s, "-") {
			return 0
		}
		s = "-" + s
	}

	value, err := strconv.ParseFloat(s, 64)
	if err != nil || math.IsNaN(value) || math.IsInf(value, 0) {
		return 0
	}
	return value
}

func isMissingNumber(raw string) bool {
	switch strings.ToUpper(strings.TrimSpace(raw)) {
	case "", "--", "N/A", "NA", "NAN", "INF", "INFINITY":
		return true
	default:
		return false
	}
}

func parsePercent(raw string) float64 {
	return parseNumber(raw)
}

func parseVolume(raw string) int64 {
	value := parseNumber(raw)
	if value <= 0 {
		return 0
	}
	const maxInt64 = float64(^uint64(0) >> 1)
	if value >= maxInt64 {
		return 0
	}
	return int64(value)
}

func firstNumber(values ...string) float64 {
	for _, value := range values {
		if parsed := parseNumber(value); parsed > 0 {
			return parsed
		}
	}
	return 0
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

var timestampLayouts = []string{
	time.RFC3339Nano,
	"2006-01-02T15:04:05.999999999Z07:00",
	"2006-01-02 15:04:05.999999999 -0700 MST",
	"2006-01-02 15:04:05",
	"01/02/2006 3:04:05 PM",
	"01/02/2006 15:04:05",
	"01/02/2006",
	"1/2/2006 3:04:05 PM",
	"1/2/2006 15:04:05",
	"1/2/2006",
	"2006-01-02",
	"Jan 2, 2006",
	"January 2, 2006",
}

var timeOnlyLayouts = []string{
	"3:04:05 PM",
	"15:04:05",
}

// chartTimestamp 优先使用 Nasdaq 图表自带的毫秒时间，dateTime 作为兜底。
func chartTimestamp(epochMillis int64, dateTime string) int64 {
	if epochMillis > 0 {
		return epochMillis
	}
	return parseTimestamp(dateTime)
}

func parseTimestamp(raw string) int64 {
	s := strings.TrimSpace(raw)
	if s == "" {
		return time.Now().UnixMilli()
	}
	if numeric, err := strconv.ParseInt(s, 10, 64); err == nil {
		switch {
		case numeric >= 100_000_000_000_000_000:
			return numeric / 1_000_000
		case numeric >= 100_000_000_000_000:
			return numeric / 1_000
		case numeric >= 1_000_000_000_000:
			return numeric
		case numeric >= 1_000_000_000:
			return numeric * 1_000
		}
	}
	// Nasdaq 有时只返回当日时间，没有日期部分；此时退回当前时间。
	for _, layout := range timeOnlyLayouts {
		if _, err := time.Parse(layout, s); err == nil {
			return time.Now().UnixMilli()
		}
	}
	for _, layout := range timestampLayouts {
		if parsed, err := time.Parse(layout, s); err == nil {
			return parsed.UTC().UnixMilli()
		}
	}
	return time.Now().UnixMilli()
}
