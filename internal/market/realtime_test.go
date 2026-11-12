package market_test

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"my-go-exchange/internal/constant"
	"my-go-exchange/internal/market"
	"my-go-exchange/internal/marketdata"
	"my-go-exchange/internal/stream"
	"my-go-exchange/internal/trading"
)

type realtimeMessage struct {
	Stream string          `json:"stream"`
	Type   string          `json:"type"`
	Seq    int64           `json:"seq"`
	Data   json.RawMessage `json:"data"`
}

type realtimeOrderData struct {
	OrderID   string         `json:"order_id"`
	State     constant.State `json:"state"`
	Remaining string         `json:"remaining"`
}

func TestLocalMarketPublishesBookTradeAndPrivateOrderEvents(t *testing.T) {
	exchange := market.NewMarket()
	local, err := exchange.RegisterLocal(&trading.Instrument{
		Symbol: "BTC/USDT",
		Base:   "BTC",
		Quote:  "USDT",
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := exchange.Deposit(context.Background(), 1, "USDT", 1000); err != nil {
		t.Fatal(err)
	}

	subscription, err := exchange.Streams().SnapshotAndSubscribe(
		context.Background(),
		stream.SubscribeRequest{
			AccountID: 1,
			Streams: []stream.StreamRequest{
				{Name: "book.BTC/USDT", Depth: 5},
				{Name: "trades.BTC/USDT"},
				{Name: "orders"},
			},
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	defer subscription.Close()
	for i := 0; i < 3; i++ {
		receiveRealtime(t, subscription.Messages)
	}

	if err := local.ApplyBook(context.Background(), marketdata.Book{
		Symbol: "BTC/USDT",
		Bids:   []marketdata.Level{{Price: 100, Amount: 10}},
		Asks:   []marketdata.Level{{Price: 101, Amount: 10}},
	}); err != nil {
		t.Fatal(err)
	}
	bookDelta := receiveRealtime(t, subscription.Messages)
	if bookDelta.Stream != "book.BTC/USDT" || bookDelta.Type != "book.delta" {
		t.Fatalf("book delta = %+v", bookDelta)
	}

	order, err := exchange.Place(context.Background(), market.PlaceReq{
		AccountID:     1,
		ClientOrderID: "realtime-take",
		Type:          constant.Limit,
		Symbol:        "BTC/USDT",
		OrderSide:     constant.Buy,
		PositionSide:  constant.Long,
		Price:         101,
		Amount:        2,
		Leverage:      1,
	})
	if err != nil {
		t.Fatal(err)
	}
	if order.State != constant.StateFilled {
		t.Fatalf("order = %+v, want filled", order)
	}

	bookDelta = receiveRealtime(t, subscription.Messages)
	trade := receiveRealtime(t, subscription.Messages)
	orderUpdate := receiveRealtime(t, subscription.Messages)
	fill := receiveRealtime(t, subscription.Messages)
	if bookDelta.Type != "book.delta" || trade.Type != "trade" ||
		orderUpdate.Type != "order.update" || fill.Type != "fill" {
		t.Fatalf("event sequence = %s, %s, %s, %s",
			bookDelta.Type, trade.Type, orderUpdate.Type, fill.Type)
	}

	book, err := exchange.Book(context.Background(), "BTC/USDT")
	if err != nil {
		t.Fatal(err)
	}
	if len(book.Asks) != 1 || book.Asks[0].Amount != 8 || book.LastUpdateID != 2 {
		t.Fatalf("canonical book = %+v", book)
	}
}

func TestLocalMarketPublishesPerTradeTakerState(t *testing.T) {
	exchange := market.NewMarket()
	if _, err := exchange.RegisterLocal(&trading.Instrument{
		Symbol: "BTC/USDT",
		Base:   "BTC",
		Quote:  "USDT",
	}); err != nil {
		t.Fatal(err)
	}
	for _, accountID := range []int64{1, 2, 3} {
		if err := exchange.Deposit(context.Background(), accountID, "USDT", 1000); err != nil {
			t.Fatal(err)
		}
	}
	for i, maker := range []market.PlaceReq{
		{
			AccountID:     1,
			ClientOrderID: "maker-1",
			Type:          constant.Limit,
			Symbol:        "BTC/USDT",
			OrderSide:     constant.Sell,
			PositionSide:  constant.Short,
			Price:         100,
			Amount:        2,
			Leverage:      1,
		},
		{
			AccountID:     2,
			ClientOrderID: "maker-2",
			Type:          constant.Limit,
			Symbol:        "BTC/USDT",
			OrderSide:     constant.Sell,
			PositionSide:  constant.Short,
			Price:         101,
			Amount:        3,
			Leverage:      1,
		},
	} {
		if _, err := exchange.Place(context.Background(), maker); err != nil {
			t.Fatalf("place maker %d: %v", i+1, err)
		}
	}

	subscription, err := exchange.Streams().SnapshotAndSubscribe(
		context.Background(),
		stream.SubscribeRequest{
			AccountID: 3,
			Streams:   []stream.StreamRequest{{Name: "orders"}},
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	defer subscription.Close()
	receiveRealtime(t, subscription.Messages)

	if _, err := exchange.Place(context.Background(), market.PlaceReq{
		AccountID:     3,
		ClientOrderID: "multi-level-taker",
		Type:          constant.Limit,
		Symbol:        "BTC/USDT",
		OrderSide:     constant.Buy,
		PositionSide:  constant.Long,
		Price:         101,
		Amount:        5,
		Leverage:      1,
	}); err != nil {
		t.Fatal(err)
	}

	firstUpdate := receiveRealtime(t, subscription.Messages)
	firstFill := receiveRealtime(t, subscription.Messages)
	secondUpdate := receiveRealtime(t, subscription.Messages)
	secondFill := receiveRealtime(t, subscription.Messages)
	if firstUpdate.Type != "order.update" || firstFill.Type != "fill" ||
		secondUpdate.Type != "order.update" || secondFill.Type != "fill" {
		t.Fatalf("event sequence = %s, %s, %s, %s",
			firstUpdate.Type, firstFill.Type, secondUpdate.Type, secondFill.Type)
	}

	var firstState realtimeOrderData
	if err := json.Unmarshal(firstUpdate.Data, &firstState); err != nil {
		t.Fatal(err)
	}
	var secondState realtimeOrderData
	if err := json.Unmarshal(secondUpdate.Data, &secondState); err != nil {
		t.Fatal(err)
	}
	if firstState.State != constant.StatePartial || firstState.Remaining != "3" {
		t.Fatalf("first order state = %+v, want partial remaining 3", firstState)
	}
	if secondState.State != constant.StateFilled || secondState.Remaining != "0" {
		t.Fatalf("second order state = %+v, want filled remaining 0", secondState)
	}
}

func receiveRealtime(t *testing.T, messages <-chan []byte) realtimeMessage {
	t.Helper()
	select {
	case payload, ok := <-messages:
		if !ok {
			t.Fatal("subscription closed before message")
		}
		var message realtimeMessage
		if err := json.Unmarshal(payload, &message); err != nil {
			t.Fatalf("decode message %q: %v", payload, err)
		}
		return message
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for realtime message")
		return realtimeMessage{}
	}
}
