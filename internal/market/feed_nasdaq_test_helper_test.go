package market

import (
	"net/http"

	"my-go-exchange/internal/market/nasdaq"
)

func newTestNasdaqSource(base string) *nasdaq.Source {
	return &nasdaq.Source{Base: base, Http: http.DefaultClient}
}
