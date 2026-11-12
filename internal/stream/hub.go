package stream

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"my-go-exchange/internal/constant"
)

var (
	ErrInvalidStream          = errors.New("stream: invalid stream")
	ErrPrivateAuthentication  = errors.New("stream: private stream requires authentication")
	ErrSlowConsumer           = errors.New("stream: subscriber is too slow")
	ErrSubscriptionClosed     = errors.New("stream: subscription closed")
	ErrHistoryNotAvailable    = errors.New("stream: requested history is unavailable")
	ErrSubscriptionQueueEmpty = errors.New("stream: subscription queue cannot hold initial event")
)

// Config controls per-connection buffering, replay depth and the maximum book
// depth served by the in-process hub.
type Config struct {
	BufferSize   int
	HistorySize  int
	DefaultDepth int
	MaxDepth     int
	MaxStreams   int
}

func (c Config) withDefaults() Config {
	if c.BufferSize <= 0 {
		c.BufferSize = 256
	}
	if c.HistorySize <= 0 {
		c.HistorySize = 4096
	}
	if c.DefaultDepth <= 0 {
		c.DefaultDepth = 20
	}
	if c.MaxDepth <= 0 {
		c.MaxDepth = 1000
	}
	if c.MaxStreams <= 0 {
		c.MaxStreams = 32
	}
	if c.DefaultDepth > c.MaxDepth {
		c.DefaultDepth = c.MaxDepth
	}
	return c
}

// SubscribeRequest identifies the account and streams requested by one
// connection.
type SubscribeRequest struct {
	AccountID int64
	Streams   []StreamRequest
}

// StreamRequest asks for one public or private stream.
type StreamRequest struct {
	Name     string `json:"name"`
	Depth    int    `json:"depth,omitempty"`
	SinceSeq int64  `json:"since_seq,omitempty"`
}

// Subscription receives pre-encoded WebSocket messages. Messages is closed
// when the subscription is explicitly closed or removed for falling behind.
type Subscription struct {
	ID       uint64
	Messages <-chan []byte

	closeOnce sync.Once
	closeFn   func()
	sub       *subscriber
}

// Close removes the subscription and closes its Messages channel.
func (s *Subscription) Close() {
	if s == nil {
		return
	}
	s.closeOnce.Do(func() {
		if s.closeFn != nil {
			s.closeFn()
		}
	})
}

// Err returns the reason a subscription ended. A nil error means it is still
// active.
func (s *Subscription) Err() error {
	if s == nil || s.sub == nil {
		return nil
	}
	return s.sub.errorValue()
}

type subscriber struct {
	id        uint64
	accountID int64
	messages  chan []byte
	books     map[*bookView]struct{}
	trades    map[*sequenceStream]struct{}
	account   *sequenceStream

	errMu sync.Mutex
	err   error
}

func (s *subscriber) setErr(err error) {
	s.errMu.Lock()
	s.err = err
	s.errMu.Unlock()
}

func (s *subscriber) errorValue() error {
	s.errMu.Lock()
	defer s.errMu.Unlock()
	return s.err
}

type storedMessage struct {
	seq         int64
	previousSeq int64
	payload     []byte
}

type bookState struct {
	symbol string
	seq    int64
	bids   map[int64]BookLevel
	asks   map[int64]BookLevel
	views  map[int]*bookView
}

type bookView struct {
	state   *bookState
	depth   int
	seq     int64
	bids    map[int64]BookLevel
	asks    map[int64]BookLevel
	history []storedMessage
	subs    map[uint64]struct{}
}

type sequenceStream struct {
	name    string
	seq     int64
	history []storedMessage
	subs    map[uint64]struct{}
}

// Hub is an in-process event hub with logical per-symbol and per-account
// streams. Publish never waits for socket I/O; a subscriber that fills its
// buffer is disconnected.
type Hub struct {
	mu sync.Mutex

	cfg      Config
	nextSub  uint64
	subs     map[uint64]*subscriber
	books    map[string]*bookState
	trades   map[string]*sequenceStream
	accounts map[int64]*sequenceStream
}

func NewHub(cfg Config) *Hub {
	return &Hub{
		cfg:      cfg.withDefaults(),
		subs:     make(map[uint64]*subscriber),
		books:    make(map[string]*bookState),
		trades:   make(map[string]*sequenceStream),
		accounts: make(map[int64]*sequenceStream),
	}
}

// Publish applies one committed symbol change and fans it out without blocking
// on subscriber network I/O.
func (h *Hub) Publish(commit Commit) {
	if h == nil || commit.Symbol == "" || commit.Empty() {
		return
	}
	if commit.Timestamp <= 0 {
		commit.Timestamp = time.Now().UnixMilli()
	}

	h.mu.Lock()
	defer h.mu.Unlock()

	if len(commit.Bids) > 0 || len(commit.Asks) > 0 {
		h.publishBookLocked(commit)
	}
	for _, trade := range commit.Trades {
		h.publishTradeLocked(commit.Symbol, commit.Timestamp, trade)
	}
	for _, event := range commit.Private {
		h.publishPrivateLocked(commit.Timestamp, event)
	}
}

func (h *Hub) publishBookLocked(commit Commit) {
	state := h.books[commit.Symbol]
	if state == nil {
		state = newBookState(commit.Symbol)
		h.books[commit.Symbol] = state
	}
	applyChanges(state.bids, commit.Bids)
	applyChanges(state.asks, commit.Asks)

	state.seq = commit.BookSequence
	if state.seq <= 0 {
		state.seq++
	}
	for _, view := range state.views {
		h.publishBookViewLocked(state, view, commit.Timestamp)
	}
}

func (h *Hub) publishBookViewLocked(state *bookState, view *bookView, timestamp int64) {
	nextBids := topLevels(state.bids, view.depth, true)
	nextAsks := topLevels(state.asks, view.depth, false)
	bidChanges := diffLevels(view.bids, nextBids, true)
	askChanges := diffLevels(view.asks, nextAsks, false)
	if len(bidChanges) == 0 && len(askChanges) == 0 {
		return
	}

	previous := view.seq
	view.seq = state.seq
	view.bids = nextBids
	view.asks = nextAsks

	payload := marshalMessage(
		"book."+state.symbol,
		"book.delta",
		view.seq,
		previous,
		timestamp,
		wireBookData{
			Symbol: state.symbol,
			Bids:   wireLevels(bidChanges),
			Asks:   wireLevels(askChanges),
		},
	)
	h.appendBookHistory(view, storedMessage{
		seq:         view.seq,
		previousSeq: previous,
		payload:     payload,
	})
	h.sendToSubscribersLocked(view.subs, payload)
}

func (h *Hub) publishTradeLocked(symbol string, timestamp int64, trade Trade) {
	topic := h.trades[symbol]
	if topic == nil {
		topic = newSequenceStream("trades." + symbol)
		h.trades[symbol] = topic
	}
	previous := topic.seq
	topic.seq++
	payload := marshalMessage(
		topic.name,
		"trade",
		topic.seq,
		previous,
		timestamp,
		wireTradeData{
			Symbol:    symbol,
			TradeID:   Fixed(trade.TradeID),
			Price:     Fixed(trade.Price),
			Amount:    Fixed(trade.Amount),
			TakerSide: trade.TakerSide,
		},
	)
	h.appendSequenceHistory(topic, storedMessage{
		seq:         topic.seq,
		previousSeq: previous,
		payload:     payload,
	})
	h.sendToSubscribersLocked(topic.subs, payload)
}

func (h *Hub) publishPrivateLocked(timestamp int64, event PrivateEvent) {
	accountID, payload := privatePayload(event)
	if accountID <= 0 || payload == nil {
		return
	}
	topic := h.accounts[accountID]
	if topic == nil {
		topic = newSequenceStream("orders")
		h.accounts[accountID] = topic
	}
	previous := topic.seq
	topic.seq++

	var message []byte
	switch event.Kind {
	case PrivateOrderUpdate:
		message = marshalMessage(topic.name, string(event.Kind), topic.seq, previous, timestamp, payload)
	case PrivateFill:
		message = marshalMessage(topic.name, string(event.Kind), topic.seq, previous, timestamp, payload)
	default:
		topic.seq--
		return
	}
	h.appendSequenceHistory(topic, storedMessage{
		seq:         topic.seq,
		previousSeq: previous,
		payload:     message,
	})
	h.sendToSubscribersLocked(topic.subs, message)
}

// SnapshotAndSubscribe registers all requested streams and queues their initial
// snapshot or replay while holding the same hub lock used by Publish.
func (h *Hub) SnapshotAndSubscribe(
	_ context.Context,
	request SubscribeRequest,
) (*Subscription, error) {
	if h == nil {
		return nil, fmt.Errorf("%w: hub is nil", ErrInvalidStream)
	}
	if len(request.Streams) == 0 {
		return nil, fmt.Errorf("%w: at least one stream is required", ErrInvalidStream)
	}
	if len(request.Streams) > h.cfg.MaxStreams {
		return nil, fmt.Errorf("%w: too many streams", ErrInvalidStream)
	}

	h.mu.Lock()
	defer h.mu.Unlock()

	h.nextSub++
	sub := &subscriber{
		id:        h.nextSub,
		accountID: request.AccountID,
		messages:  make(chan []byte, h.cfg.BufferSize),
		books:     make(map[*bookView]struct{}),
		trades:    make(map[*sequenceStream]struct{}),
	}
	h.subs[sub.id] = sub

	for _, streamRequest := range request.Streams {
		if err := h.subscribeLocked(sub, streamRequest); err != nil {
			h.removeSubscriberLocked(sub, err)
			return nil, err
		}
	}

	subscription := &Subscription{
		ID:       sub.id,
		Messages: sub.messages,
		sub:      sub,
	}
	subscription.closeFn = func() {
		h.unsubscribe(sub.id, ErrSubscriptionClosed)
	}
	return subscription, nil
}

func (h *Hub) subscribeLocked(sub *subscriber, request StreamRequest) error {
	switch {
	case request.Name == "orders":
		if sub.accountID <= 0 {
			return ErrPrivateAuthentication
		}
		topic := h.accounts[sub.accountID]
		if topic == nil {
			topic = newSequenceStream("orders")
			h.accounts[sub.accountID] = topic
		}
		sub.account = topic
		topic.subs[sub.id] = struct{}{}
		return h.queueSequenceInitialLocked(sub, topic, request.SinceSeq)

	case strings.HasPrefix(request.Name, "trades."):
		symbol := strings.TrimPrefix(request.Name, "trades.")
		if symbol == "" {
			return fmt.Errorf("%w: symbol is required", ErrInvalidStream)
		}
		topic := h.trades[symbol]
		if topic == nil {
			topic = newSequenceStream("trades." + symbol)
			h.trades[symbol] = topic
		}
		sub.trades[topic] = struct{}{}
		topic.subs[sub.id] = struct{}{}
		return h.queueSequenceInitialLocked(sub, topic, request.SinceSeq)

	case strings.HasPrefix(request.Name, "book."):
		symbol := strings.TrimPrefix(request.Name, "book.")
		if symbol == "" {
			return fmt.Errorf("%w: symbol is required", ErrInvalidStream)
		}
		depth := request.Depth
		if depth <= 0 {
			depth = h.cfg.DefaultDepth
		}
		if depth > h.cfg.MaxDepth {
			return fmt.Errorf("%w: depth exceeds %d", ErrInvalidStream, h.cfg.MaxDepth)
		}
		state := h.books[symbol]
		if state == nil {
			state = newBookState(symbol)
			h.books[symbol] = state
		}
		view := state.views[depth]
		if view == nil {
			view = newBookView(state, depth)
			state.views[depth] = view
		}
		sub.books[view] = struct{}{}
		view.subs[sub.id] = struct{}{}
		return h.queueBookInitialLocked(sub, view, request.SinceSeq)

	default:
		return fmt.Errorf("%w: unknown stream %q", ErrInvalidStream, request.Name)
	}
}

func (h *Hub) queueBookInitialLocked(sub *subscriber, view *bookView, since int64) error {
	var messages [][]byte
	if since > 0 {
		if replay, ok := replayHistory(view.history, since, view.seq); ok {
			if len(replay) <= h.cfg.BufferSize {
				messages = replay
			}
		}
	}
	if messages == nil {
		if since > 0 && since == view.seq {
			return nil
		}
		payload := marshalMessage(
			"book."+view.state.symbol,
			"book.snapshot",
			view.seq,
			0,
			time.Now().UnixMilli(),
			wireBookData{
				Symbol: view.state.symbol,
				Bids:   wireLevelMap(view.bids, true),
				Asks:   wireLevelMap(view.asks, false),
			},
		)
		messages = [][]byte{payload}
	}
	for _, message := range messages {
		select {
		case sub.messages <- message:
		default:
			return ErrSubscriptionQueueEmpty
		}
	}
	return nil
}

func (h *Hub) queueSequenceInitialLocked(
	sub *subscriber,
	topic *sequenceStream,
	since int64,
) error {
	if since > 0 && since == topic.seq {
		return nil
	}
	if replay, ok := replayHistory(topic.history, since, topic.seq); ok && since > 0 {
		if len(replay) <= h.cfg.BufferSize {
			for _, message := range replay {
				select {
				case sub.messages <- message:
				default:
					return ErrSubscriptionQueueEmpty
				}
			}
			return nil
		}
	}

	payload := marshalMessage(
		topic.name,
		"resync.required",
		topic.seq,
		0,
		time.Now().UnixMilli(),
		map[string]string{"reason": ErrHistoryNotAvailable.Error()},
	)
	select {
	case sub.messages <- payload:
		return nil
	default:
		return ErrSubscriptionQueueEmpty
	}
}

func (h *Hub) unsubscribe(id uint64, reason error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	sub := h.subs[id]
	if sub == nil {
		return
	}
	h.removeSubscriberLocked(sub, reason)
}

func (h *Hub) removeSubscriberLocked(sub *subscriber, reason error) {
	if _, ok := h.subs[sub.id]; !ok {
		return
	}
	delete(h.subs, sub.id)
	for view := range sub.books {
		delete(view.subs, sub.id)
	}
	for topic := range sub.trades {
		delete(topic.subs, sub.id)
	}
	if sub.account != nil {
		delete(sub.account.subs, sub.id)
	}
	sub.setErr(reason)
	close(sub.messages)
}

func (h *Hub) sendToSubscribersLocked(subscribers map[uint64]struct{}, payload []byte) {
	for id := range subscribers {
		sub := h.subs[id]
		if sub == nil {
			delete(subscribers, id)
			continue
		}
		select {
		case sub.messages <- payload:
		default:
			h.removeSubscriberLocked(sub, ErrSlowConsumer)
		}
	}
}

func (h *Hub) appendBookHistory(view *bookView, message storedMessage) {
	view.history = appendHistory(view.history, message, h.cfg.HistorySize)
}

func (h *Hub) appendSequenceHistory(topic *sequenceStream, message storedMessage) {
	topic.history = appendHistory(topic.history, message, h.cfg.HistorySize)
}

func appendHistory(history []storedMessage, message storedMessage, limit int) []storedMessage {
	if limit <= 0 {
		return nil
	}
	history = append(history, message)
	if len(history) > limit {
		copy(history, history[len(history)-limit:])
		history = history[:limit]
	}
	return history
}

func replayHistory(history []storedMessage, since, current int64) ([][]byte, bool) {
	if since < 0 || since > current {
		return nil, false
	}
	if since == current {
		return [][]byte{}, true
	}
	start := -1
	for i, message := range history {
		if message.seq > since {
			if message.previousSeq != since {
				return nil, false
			}
			start = i
			break
		}
	}
	if start < 0 {
		return nil, false
	}

	messages := make([][]byte, 0, len(history)-start)
	expected := since
	for _, message := range history[start:] {
		if message.previousSeq != expected {
			return nil, false
		}
		messages = append(messages, message.payload)
		expected = message.seq
	}
	if expected != current {
		return nil, false
	}
	return messages, true
}

func newBookState(symbol string) *bookState {
	return &bookState{
		symbol: symbol,
		bids:   make(map[int64]BookLevel),
		asks:   make(map[int64]BookLevel),
		views:  make(map[int]*bookView),
	}
}

func newBookView(state *bookState, depth int) *bookView {
	return &bookView{
		state:   state,
		depth:   depth,
		seq:     state.seq,
		bids:    topLevels(state.bids, depth, true),
		asks:    topLevels(state.asks, depth, false),
		history: make([]storedMessage, 0, 64),
		subs:    make(map[uint64]struct{}),
	}
}

func newSequenceStream(name string) *sequenceStream {
	return &sequenceStream{
		name:    name,
		history: make([]storedMessage, 0, 64),
		subs:    make(map[uint64]struct{}),
	}
}

func applyChanges(levels map[int64]BookLevel, changes []BookLevel) {
	for _, change := range changes {
		if change.Amount <= 0 || change.OrderCount <= 0 {
			delete(levels, change.Price)
			continue
		}
		levels[change.Price] = change
	}
}

func topLevels(levels map[int64]BookLevel, depth int, descending bool) map[int64]BookLevel {
	if depth <= 0 || len(levels) == 0 {
		return make(map[int64]BookLevel)
	}
	prices := make([]int64, 0, len(levels))
	for price := range levels {
		prices = append(prices, price)
	}
	sort.Slice(prices, func(i, j int) bool {
		if descending {
			return prices[i] > prices[j]
		}
		return prices[i] < prices[j]
	})
	if len(prices) > depth {
		prices = prices[:depth]
	}
	result := make(map[int64]BookLevel, len(prices))
	for _, price := range prices {
		result[price] = levels[price]
	}
	return result
}

func diffLevels(
	previous map[int64]BookLevel,
	next map[int64]BookLevel,
	descending bool,
) []BookLevel {
	byPrice := make(map[int64]BookLevel)
	for price, level := range previous {
		if current, ok := next[price]; !ok || current != level {
			byPrice[price] = BookLevel{Price: price}
		}
	}
	for price, level := range next {
		if current, ok := previous[price]; !ok || current != level {
			byPrice[price] = level
		}
	}
	changes := make([]BookLevel, 0, len(byPrice))
	for _, level := range byPrice {
		changes = append(changes, level)
	}
	sort.Slice(changes, func(i, j int) bool {
		if descending {
			return changes[i].Price > changes[j].Price
		}
		return changes[i].Price < changes[j].Price
	})
	return changes
}

type wireMessage struct {
	Stream      string `json:"stream"`
	Type        string `json:"type"`
	Seq         int64  `json:"seq"`
	PreviousSeq int64  `json:"previous_seq,omitempty"`
	Timestamp   int64  `json:"timestamp"`
	Data        any    `json:"data,omitempty"`
}

type wireLevel struct {
	Price      Fixed `json:"price"`
	Amount     Fixed `json:"amount"`
	OrderCount int64 `json:"order_count,omitempty"`
}

type wireBookData struct {
	Symbol string      `json:"symbol"`
	Bids   []wireLevel `json:"bids"`
	Asks   []wireLevel `json:"asks"`
}

type wireTradeData struct {
	Symbol    string             `json:"symbol"`
	TradeID   Fixed              `json:"trade_id"`
	Price     Fixed              `json:"price"`
	Amount    Fixed              `json:"amount"`
	TakerSide constant.OrderSide `json:"taker_side"`
}

type wireOrderData struct {
	OrderID       Fixed                 `json:"order_id"`
	ClientOrderID string                `json:"client_order_id,omitempty"`
	Symbol        string                `json:"symbol"`
	OrderSide     constant.OrderSide    `json:"order_side"`
	PositionSide  constant.PositionSide `json:"position_side"`
	Type          constant.OrderType    `json:"type"`
	Price         Fixed                 `json:"price"`
	Amount        Fixed                 `json:"amount"`
	Filled        Fixed                 `json:"filled"`
	Remaining     Fixed                 `json:"remaining"`
	Leverage      int64                 `json:"leverage"`
	State         constant.State        `json:"state"`
	MatchSeq      int64                 `json:"match_seq"`
}

type wireFillData struct {
	OrderID       Fixed                 `json:"order_id"`
	ClientOrderID string                `json:"client_order_id,omitempty"`
	TradeID       Fixed                 `json:"trade_id"`
	Symbol        string                `json:"symbol"`
	OrderSide     constant.OrderSide    `json:"order_side"`
	PositionSide  constant.PositionSide `json:"position_side"`
	Role          constant.TradeRole    `json:"role"`
	Price         Fixed                 `json:"price"`
	Amount        Fixed                 `json:"amount"`
}

func marshalMessage(stream, kind string, seq, previous, timestamp int64, data any) []byte {
	payload, err := json.Marshal(wireMessage{
		Stream:      stream,
		Type:        kind,
		Seq:         seq,
		PreviousSeq: previous,
		Timestamp:   timestamp,
		Data:        data,
	})
	if err != nil {
		return nil
	}
	return payload
}

func wireLevels(levels []BookLevel) []wireLevel {
	result := make([]wireLevel, 0, len(levels))
	for _, level := range levels {
		result = append(result, wireLevel{
			Price:      Fixed(level.Price),
			Amount:     Fixed(level.Amount),
			OrderCount: level.OrderCount,
		})
	}
	return result
}

func wireLevelMap(levels map[int64]BookLevel, descending bool) []wireLevel {
	ordered := topLevels(levels, len(levels), descending)
	values := make([]BookLevel, 0, len(ordered))
	for _, level := range ordered {
		values = append(values, level)
	}
	sort.Slice(values, func(i, j int) bool {
		if descending {
			return values[i].Price > values[j].Price
		}
		return values[i].Price < values[j].Price
	})
	return wireLevels(values)
}

func privatePayload(event PrivateEvent) (int64, any) {
	switch event.Kind {
	case PrivateOrderUpdate:
		if event.Order == nil || event.Order.AccountID <= 0 {
			return 0, nil
		}
		order := event.Order
		return order.AccountID, wireOrderData{
			OrderID:       Fixed(order.OrderID),
			ClientOrderID: order.ClientOrderID,
			Symbol:        order.Symbol,
			OrderSide:     order.OrderSide,
			PositionSide:  order.PositionSide,
			Type:          order.Type,
			Price:         Fixed(order.Price),
			Amount:        Fixed(order.Amount),
			Filled:        Fixed(order.Amount - order.Remaining),
			Remaining:     Fixed(order.Remaining),
			Leverage:      order.Leverage,
			State:         order.State,
			MatchSeq:      order.Sequence,
		}
	case PrivateFill:
		if event.Fill == nil || event.Fill.AccountID <= 0 {
			return 0, nil
		}
		fill := event.Fill
		return fill.AccountID, wireFillData{
			OrderID:       Fixed(fill.OrderID),
			ClientOrderID: fill.ClientOrderID,
			TradeID:       Fixed(fill.TradeID),
			Symbol:        fill.Symbol,
			OrderSide:     fill.OrderSide,
			PositionSide:  fill.PositionSide,
			Role:          fill.Role,
			Price:         Fixed(fill.Price),
			Amount:        Fixed(fill.Amount),
		}
	default:
		return 0, nil
	}
}
