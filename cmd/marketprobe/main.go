package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"time"

	"my-go-exchange/internal/market/gate"
)

func main() {
	external := flag.String("pair", "NVDAON_USDT", "Gate spot currency pair")
	symbol := flag.String("symbol", "NVDA/USDT", "internal symbol written into the book")
	timeout := flag.Duration("timeout", 25*time.Second, "request timeout")
	flag.Parse()

	ctx, cancel := context.WithTimeout(context.Background(), *timeout)
	defer cancel()

	source := gate.NewSourceFor(*external)
	book, err := source.FetchOrderBook(ctx)
	if err != nil {
		log.Fatal(err)
	}
	book.Symbol = *symbol

	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(book); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
