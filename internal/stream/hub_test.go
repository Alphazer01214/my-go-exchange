package stream

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"my-go-exchange/internal/constant"
)

type decodedMessage struct {
	Stream      string          `json:"stream"`
	Type        string          `json:"type"`
	Seq         int64           `json:"seq"`
	PreviousSeq int64           `json:"previous_seq"`
	Timestamp   int64           `json:"timestamp"`
	Data        json.RawMessage `json:"data"`
}

type decodedLevel struct {
	Price      string `json:"price"`
	Amount     string `json:"amount"`
	OrderCount int64  `json:"order_count"`
}

type decodedBook struct {
	Symbol string         `json:"symbol"`
	Bids   []decodedLevel `json:"bids"`
	Asks   []decodedLevel `json:"asks"`
}

type decodedOrder struct {
	OrderID   string         `json:"order_id"`
	State     constant.State `json:"state"`
	Remaining string         `json:"remaining"`
}

func TestHubPublishesBookSnapshotAndDepthDelta(t *testing.T) {
	hub := NewHub(Config{BufferSize: 8, HistorySize: 8})
	sub := subscribe(t, hub, SubscribeRequest{
		Streams: []StreamRequest{{Name: "book.BTC/USDT", Depth: 2}},
	})

	snapshot := receiveMessage(t, sub.Messages)
	if snapshot.Type != "book.snapshot" || snapshot.Seq != 0 ||
		snapshot.Stream != "book.BTC/USDT" {
		t.Fatalf("snapshot = %+v", snapshot)
	}

	hub.Publish(Commit{
		Symbol:       "BTC/USDT",
		BookSequence: 1,
		Timestamp:    100,
		Bids: []BookLevel{
			{Price: 100, Amount: 5, OrderCount: 1},
			{Price: 99, Amount: 3, OrderCount: 1},
			{Price: 98, Amount: 2, OrderCount: 1},
		},
		Asks: []BookLevel{
			{Price: 101, Amount: 4, OrderCount: 1},
		},
	})
	delta := receiveMessage(t, sub.Messages)
	if delta.Type != "book.delta" || delta.Seq != 1 || delta.PreviousSeq != 0 {
		t.Fatalf("delta = %+v", delta)
	}
	var book decodedBook
	decodeData(t, delta, &book)
	if len(book.Bids) != 2 || book.Bids[0].Price != "100" || book.Bids[1].Price != "99" {
		t.Fatalf("depth-limited bids = %+v", book.Bids)
	}

	hub.Publish(Commit{
		Symbol:       "BTC/USDT",
		BookSequence: 2,
		Timestamp:    101,
		Bids: []BookLevel{
			{Price: 100, Amount: 0, OrderCount: 0},
		},
	})
	delta = receiveMessage(t, sub.Messages)
	book = decodedBook{}
	decodeData(t, delta, &book)
	if len(book.Bids) != 2 {
		t.Fatalf("boundary delta = %+v, want removal and replacement", book.Bids)
	}
	if book.Bids[0].Price != "100" || book.Bids[0].Amount != "0" ||
		book.Bids[1].Price != "98" || book.Bids[1].Amount != "2" {
		t.Fatalf("boundary delta bids = %+v", book.Bids)
	}
}

func TestHubKeepsPrivateStreamsIsolatedByAccount(t *testing.T) {
	hub := NewHub(Config{BufferSize: 4})
	sub := subscribe(t, hub, SubscribeRequest{
		AccountID: 1,
		Streams:   []StreamRequest{{Name: "orders"}},
	})
	if initial := receiveMessage(t, sub.Messages); initial.Type != "resync.required" {
		t.Fatalf("initial private message = %+v", initial)
	}

	hub.Publish(Commit{
		Symbol: "BTC/USDT",
		Private: []PrivateEvent{{
			Kind: PrivateOrderUpdate,
			Order: &OrderUpdate{
				AccountID: 2,
				OrderID:   20,
				Symbol:    "BTC/USDT",
				State:     constant.StateInBook,
			},
		}},
	})
	select {
	case message := <-sub.Messages:
		t.Fatalf("account 1 received another account's order: %s", message)
	default:
	}

	hub.Publish(Commit{
		Symbol: "BTC/USDT",
		Private: []PrivateEvent{{
			Kind: PrivateOrderUpdate,
			Order: &OrderUpdate{
				AccountID: 1,
				OrderID:   10,
				Symbol:    "BTC/USDT",
				Amount:    5,
				Remaining: 5,
				State:     constant.StateInBook,
			},
		}},
	})
	message := receiveMessage(t, sub.Messages)
	if message.Stream != "orders" || message.Type != "order.update" || message.Seq != 1 {
		t.Fatalf("private message = %+v", message)
	}
	var order decodedOrder
	decodeData(t, message, &order)
	if order.OrderID != "10" || order.State != constant.StateInBook || order.Remaining != "5" {
		t.Fatalf("private order = %+v", order)
	}
}

func TestHubReplaysTradeHistory(t *testing.T) {
	hub := NewHub(Config{BufferSize: 4, HistorySize: 8})
	first := subscribe(t, hub, SubscribeRequest{
		Streams: []StreamRequest{{Name: "trades.BTC/USDT"}},
	})
	if initial := receiveMessage(t, first.Messages); initial.Type != "resync.required" {
		t.Fatalf("initial trade message = %+v", initial)
	}
	for i := int64(1); i <= 2; i++ {
		hub.Publish(Commit{
			Symbol: "BTC/USDT",
			Trades: []Trade{{
				TradeID:   i,
				Price:     100,
				Amount:    i,
				TakerSide: constant.Buy,
			}},
		})
		if message := receiveMessage(t, first.Messages); message.Seq != i {
			t.Fatalf("trade seq = %d, want %d", message.Seq, i)
		}
	}
	first.Close()

	reconnected := subscribe(t, hub, SubscribeRequest{
		Streams: []StreamRequest{{Name: "trades.BTC/USDT", SinceSeq: 1}},
	})
	replayed := receiveMessage(t, reconnected.Messages)
	if replayed.Type != "trade" || replayed.Seq != 2 || replayed.PreviousSeq != 1 {
		t.Fatalf("replayed trade = %+v", replayed)
	}
}

func TestHubFallsBackToResyncWhenHistoryExceedsBuffer(t *testing.T) {
	hub := NewHub(Config{BufferSize: 2, HistorySize: 8})
	for i := int64(1); i <= 4; i++ {
		hub.Publish(Commit{
			Symbol: "BTC/USDT",
			Trades: []Trade{{
				TradeID:   i,
				Price:     100,
				Amount:    i,
				TakerSide: constant.Buy,
			}},
		})
	}

	sub := subscribe(t, hub, SubscribeRequest{
		Streams: []StreamRequest{{Name: "trades.BTC/USDT", SinceSeq: 1}},
	})
	message := receiveMessage(t, sub.Messages)
	if message.Type != "resync.required" || message.Seq != 4 {
		t.Fatalf("long replay response = %+v, want resync at latest seq", message)
	}
}

func TestHubRemovesSlowSubscriberWithoutBlockingPublisher(t *testing.T) {
	hub := NewHub(Config{BufferSize: 1, HistorySize: 4})
	sub := subscribe(t, hub, SubscribeRequest{
		AccountID: 1,
		Streams:   []StreamRequest{{Name: "orders"}},
	})

	done := make(chan struct{})
	go func() {
		defer close(done)
		for i := int64(1); i <= 3; i++ {
			hub.Publish(Commit{
				Symbol: "BTC/USDT",
				Private: []PrivateEvent{{
					Kind: PrivateOrderUpdate,
					Order: &OrderUpdate{
						AccountID: 1,
						OrderID:   i,
						Symbol:    "BTC/USDT",
					},
				}},
			})
		}
	}()

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("publisher blocked on slow subscriber")
	}
	if !errors.Is(sub.Err(), ErrSlowConsumer) {
		t.Fatalf("subscription error = %v, want slow consumer", sub.Err())
	}
}

func subscribe(t *testing.T, hub *Hub, request SubscribeRequest) *Subscription {
	t.Helper()
	subscription, err := hub.SnapshotAndSubscribe(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(subscription.Close)
	return subscription
}

func receiveMessage(t *testing.T, messages <-chan []byte) decodedMessage {
	t.Helper()
	select {
	case payload, ok := <-messages:
		if !ok {
			t.Fatal("subscription closed before message")
		}
		var message decodedMessage
		if err := json.Unmarshal(payload, &message); err != nil {
			t.Fatalf("decode message %q: %v", payload, err)
		}
		return message
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for message")
		return decodedMessage{}
	}
}

func decodeData(t *testing.T, message decodedMessage, target any) {
	t.Helper()
	if err := json.Unmarshal(message.Data, target); err != nil {
		t.Fatalf("decode data %q: %v", message.Data, err)
	}
}
