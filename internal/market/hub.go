package market

import (
	"sort"
	"sync"
	"time"

	"my-go-exchange/internal/md"
)

// Bar is a 1-minute OHLCV candle.
type Bar struct {
	TS     int64   `json:"ts"`
	Open   float64 `json:"open"`
	High   float64 `json:"high"`
	Low    float64 `json:"low"`
	Close  float64 `json:"close"`
	Volume int64   `json:"volume"`
}

// TapeItem is a compact recent tick for the trade tape.
type TapeItem struct {
	InstrumentID string   `json:"instrument_id"`
	Price        *float64 `json:"price"`
	Volume       int64    `json:"volume"`
	Bid          *float64 `json:"bid"`
	Ask          *float64 `json:"ask"`
	UpdateTime   string   `json:"update_time"`
	Timestamp    float64  `json:"timestamp"`
}

// Event is pushed to WebSocket subscribers.
type Event struct {
	Type string `json:"type"`
	Data any    `json:"data,omitempty"`
}

const (
	tapeLimit  = 200
	barLimit   = 240
	subBufSize = 256
)

type instrumentState struct {
	last md.Tick
	bars []Bar
	tape []TapeItem
}

// Hub caches live quotes and fans out events to UI clients.
type Hub struct {
	mu    sync.RWMutex
	inst  map[string]*instrumentState
	subs  map[chan Event]struct{}
	total int64
}

func NewHub() *Hub {
	return &Hub{
		inst: make(map[string]*instrumentState),
		subs: make(map[chan Event]struct{}),
	}
}

func (h *Hub) Subscribe() (<-chan Event, func()) {
	ch := make(chan Event, subBufSize)
	h.mu.Lock()
	h.subs[ch] = struct{}{}
	h.mu.Unlock()
	unsub := func() {
		h.mu.Lock()
		if _, ok := h.subs[ch]; ok {
			delete(h.subs, ch)
			close(ch)
		}
		h.mu.Unlock()
	}
	return ch, unsub
}

func (h *Hub) broadcast(ev Event) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	for ch := range h.subs {
		select {
		case ch <- ev:
		default:
		}
	}
}

// OnTick updates caches and broadcasts tick/kline events.
func (h *Hub) OnTick(t md.Tick) {
	if t.InstrumentID == "" {
		return
	}
	price := 0.0
	if t.Price != nil {
		price = *t.Price
	}

	h.mu.Lock()
	st := h.inst[t.InstrumentID]
	if st == nil {
		st = &instrumentState{}
		h.inst[t.InstrumentID] = st
	}
	st.last = t
	st.tape = append(st.tape, TapeItem{
		InstrumentID: t.InstrumentID,
		Price:        t.Price,
		Volume:       t.Volume,
		Bid:          t.Bid,
		Ask:          t.Ask,
		UpdateTime:   t.UpdateTime,
		Timestamp:    t.Timestamp,
	})
	if len(st.tape) > tapeLimit {
		st.tape = st.tape[len(st.tape)-tapeLimit:]
	}
	bar := upsertBar(st, t, price)
	h.total++
	h.mu.Unlock()

	h.broadcast(Event{Type: "tick", Data: t})
	if bar != nil {
		h.broadcast(Event{Type: "kline", Data: map[string]any{
			"instrument_id": t.InstrumentID,
			"bar":           bar,
		}})
	}
}

func upsertBar(st *instrumentState, t md.Tick, price float64) *Bar {
	if price <= 0 {
		return nil
	}
	ts := time.Now().Unix() / 60 * 60
	if len(st.bars) == 0 || st.bars[len(st.bars)-1].TS != ts {
		st.bars = append(st.bars, Bar{
			TS:     ts,
			Open:   price,
			High:   price,
			Low:    price,
			Close:  price,
			Volume: t.Volume,
		})
	} else {
		b := &st.bars[len(st.bars)-1]
		if price > b.High {
			b.High = price
		}
		if price < b.Low {
			b.Low = price
		}
		b.Close = price
		b.Volume = t.Volume
	}
	if len(st.bars) > barLimit {
		st.bars = st.bars[len(st.bars)-barLimit:]
	}
	bar := st.bars[len(st.bars)-1]
	return &bar
}

// Snapshot returns latest quotes, tape and klines.
func (h *Hub) Snapshot() (quotes []md.Tick, tape []TapeItem, klines map[string][]Bar, total int64) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	quotes = make([]md.Tick, 0, len(h.inst))
	klines = make(map[string][]Bar, len(h.inst))
	tape = make([]TapeItem, 0, tapeLimit)
	for id, st := range h.inst {
		quotes = append(quotes, st.last)
		bars := make([]Bar, len(st.bars))
		copy(bars, st.bars)
		klines[id] = bars
		tape = append(tape, st.tape...)
	}
	sort.Slice(tape, func(i, j int) bool {
		if tape[i].Timestamp == tape[j].Timestamp {
			return tape[i].InstrumentID < tape[j].InstrumentID
		}
		return tape[i].Timestamp < tape[j].Timestamp
	})
	if len(tape) > tapeLimit {
		tape = tape[len(tape)-tapeLimit:]
	}
	return quotes, tape, klines, h.total
}

// Quote returns one instrument's latest tick.
func (h *Hub) Quote(id string) (md.Tick, bool) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	st, ok := h.inst[id]
	if !ok {
		return md.Tick{}, false
	}
	return st.last, true
}

// Klines returns bars for one instrument.
func (h *Hub) Klines(id string) []Bar {
	h.mu.RLock()
	defer h.mu.RUnlock()
	st, ok := h.inst[id]
	if !ok {
		return nil
	}
	out := make([]Bar, len(st.bars))
	copy(out, st.bars)
	return out
}
