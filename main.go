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
	engine := matching.NewEngine(btcusdt)
	in := make(chan matching.Command, 1024)
	out := make(chan matching.Event, 1024)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go func() {
		engine.Run()
	}()

}
