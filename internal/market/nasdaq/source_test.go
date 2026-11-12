package nasdaq

import (
	"context"
	"math"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestParseNumber(t *testing.T) {
	tests := []struct {
		raw  string
		want float64
	}{
		{raw: "$1,234.50", want: 1234.5},
		{raw: "(2.5)", want: -2.5},
		{raw: "+1.25%", want: 1.25},
		{raw: "--", want: 0},
		{raw: "N/A", want: 0},
		{raw: "1x2", want: 0},
		{raw: "1..2", want: 0},
	}
	for _, tt := range tests {
		if got := parseNumber(tt.raw); got != tt.want {
			t.Errorf("parseNumber(%q) = %v, want %v", tt.raw, got, tt.want)
		}
	}
}

func TestNormalizeQuoteLiquidity(t *testing.T) {
	t.Run("cheap instrument", func(t *testing.T) {
		q := &Quote{Last: 0.01}
		normalizeQuoteLiquidity(q)
		if q.Bid <= 0 || q.Ask <= q.Bid {
			t.Fatalf("bid/ask = %v/%v, want positive non-crossed spread", q.Bid, q.Ask)
		}
	})

	t.Run("crossed quote", func(t *testing.T) {
		q := &Quote{Last: 100, Bid: 101, Ask: 99}
		normalizeQuoteLiquidity(q)
		if q.Bid >= 100 || q.Ask <= 100 {
			t.Fatalf("bid/ask = %v/%v, want synthetic spread around 100", q.Bid, q.Ask)
		}
	})
}

func TestFetchQuoteParsesFormattedFields(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/quote/AAPL/info" {
			t.Errorf("path = %q", r.URL.Path)
		}
		if got := r.URL.Query().Get("assetclass"); got != "stocks" {
			t.Errorf("assetclass = %q, want stocks", got)
		}
		_, _ = w.Write([]byte(`{
			"data": {
				"symbol": "AAPL",
				"companyName": "Apple Inc.",
				"lastTradeTimestamp": "01/02/2026 3:04:05 PM",
				"primaryData": {
					"lastSalePrice": "$101.25",
					"netChange": "+1.25",
					"percentageChange": "1.25%",
					"bidPrice": "101.20",
					"askPrice": "101.30",
					"volume": "1,234",
					"openPrice": "$100.00"
				},
				"secondaryData": {
					"previousClose": "$100.00",
					"todaysHigh": "$102.00",
					"todaysLow": "$99.50",
					"shareVolume": "5,678"
				}
			}
		}`))
	}))
	defer server.Close()

	src := NewSource()
	src.Base = server.URL
	q, err := src.FetchQuote(context.Background(), "AAPL")
	if err != nil {
		t.Fatalf("FetchQuote: %v", err)
	}
	if q.Symbol != "AAPL" || q.Name != "Apple Inc." {
		t.Fatalf("quote identity = %+v", q)
	}
	if q.Last != 101.25 || q.Bid != 101.2 || q.Ask != 101.3 {
		t.Fatalf("prices = %+v", q)
	}
	if q.Open != 100 || q.High != 102 || q.Low != 99.5 || q.PrevClose != 100 {
		t.Fatalf("OHLC = %+v", q)
	}
	if q.Volume != 1234 {
		t.Fatalf("volume = %d, want 1234", q.Volume)
	}
	if q.Timestamp <= 0 {
		t.Fatalf("timestamp = %d", q.Timestamp)
	}
}

func TestFetchQuoteEnrichesFromChart(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/quote/EMPTY/info":
			_, _ = w.Write([]byte(`{"data":{"symbol":"EMPTY","primaryData":{},"secondaryData":{}}}`))
		case "/api/quote/EMPTY/chart":
			_, _ = w.Write([]byte(`{"data":{"chart":[
				{"x":3000,"z":{"open":"11.5","high":"12.5","low":"11.0","close":"12.0","volume":"100"}},
				{"x":2000,"z":{"open":"10.5","high":"11.5","low":"10.0","close":"11.0","volume":"90"}},
				{"x":0,"z":{"open":"bad","close":"--"}}
			]}}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	src := NewSource()
	src.Base = server.URL
	q, err := src.FetchQuote(context.Background(), "EMPTY")
	if err != nil {
		t.Fatalf("FetchQuote: %v", err)
	}
	if q.Last != 12 || q.Open != 11.5 || q.High != 12.5 || q.Low != 11 || q.PrevClose != 11 {
		t.Fatalf("enriched quote = %+v", q)
	}
	if q.Bid <= 0 || q.Ask <= q.Bid {
		t.Fatalf("synthetic bid/ask = %v/%v", q.Bid, q.Ask)
	}
}

func TestFetchQuoteErrors(t *testing.T) {
	t.Run("status", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			http.Error(w, "busy", http.StatusServiceUnavailable)
		}))
		defer server.Close()
		src := NewSource()
		src.Base = server.URL
		_, err := src.FetchQuote(context.Background(), "AAPL")
		if err == nil || !strings.Contains(err.Error(), "status 503") {
			t.Fatalf("error = %v", err)
		}
	})

	t.Run("no usable price", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if strings.HasSuffix(r.URL.Path, "/chart") {
				_, _ = w.Write([]byte(`{"data":{"chart":[]}}`))
				return
			}
			_, _ = w.Write([]byte(`{"data":{"primaryData":{},"secondaryData":{}}}`))
		}))
		defer server.Close()
		src := NewSource()
		src.Base = server.URL
		_, err := src.FetchQuote(context.Background(), "AAPL")
		if err == nil || !strings.Contains(err.Error(), "no usable price") {
			t.Fatalf("error = %v", err)
		}
	})
}

func TestFetchKlineFiltersMalformedRows(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"data":{"chart":[
			{"x":2000,"z":{"open":"1","high":"2","low":"0.5","close":"1.5","volume":"10"}},
			{"x":0,"z":{"close":"bad"}},
			{"x":1000,"z":{"open":"0.5","high":"1","low":"0.25","close":"1.0","volume":"5"}}
		]}}`))
	}))
	defer server.Close()

	src := NewSource()
	src.Base = server.URL
	klines, err := src.FetchKline(context.Background(), "AAPL", 5)
	if err != nil {
		t.Fatalf("FetchKline: %v", err)
	}
	if len(klines) != 2 {
		t.Fatalf("klines = %+v, want two valid rows", klines)
	}
	if math.Abs(klines[0].Close-1.5) > 1e-9 {
		t.Fatalf("close = %v, want 1.5", klines[0].Close)
	}
}
