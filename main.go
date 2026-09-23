package main

import (
	"context"
	"my-go-exchange/internal/matching"
	"my-go-exchange/internal/types"
)

func main() {
	btcusdt := &types.Instrument{
		Symbol: "BTC/USDT",
		Base:   "BTC",
		Quote:  "USDT",
	}
	_ = matching.NewEngine(btcusdt)
	_ = make(chan matching.Command, 1024)
	_ = make(chan matching.Event, 1024)
	_, cancel := context.WithCancel(context.Background())
	defer cancel()
}
