package market

import (
	"context"
	"fmt"
	"my-go-exchange/internal/account"
	"my-go-exchange/internal/constant"
	"my-go-exchange/internal/entity"
	"my-go-exchange/internal/marketdata"
	"my-go-exchange/internal/matching"
	"my-go-exchange/internal/orderbook"
	"my-go-exchange/internal/portfolio"
	"my-go-exchange/internal/store"
	"my-go-exchange/internal/stream"
	"my-go-exchange/internal/trading"
	"sync"
	"time"
)

var _ SymbolMarket = (*LocalSymbolMarket)(nil)

type LocalSymbolMarket struct {
	mu sync.Mutex

	symbol   string
	engine   *matching.Engine
	inst     *trading.Instrument
	account  *account.Ledger
	position *portfolio.Manager
	store    *store.Store
	mirror   *Mirror
	lastBook marketdata.Book
	events   EventPublisher
	bookSeq  int64

	// orders  map[int64]trading.Order // order_id -> order
	clients        map[string]int64 // "account_id-client_order_id" -> order_id
	liquidationSeq int64
}

func NewLocalSymbolMarket(
	inst *trading.Instrument,
	account *account.Ledger,
	position *portfolio.Manager,
	store *store.Store,
	ids *matching.IDAllocator,
	events EventPublisher,
) *LocalSymbolMarket {
	if inst == nil {
		return nil
	}
	if store != nil && store.GetInst(inst.Symbol) == nil {
		store.SaveInst(inst)
	}

	engine := matching.NewEngine(inst, ids)
	return &LocalSymbolMarket{
		symbol:   inst.Symbol,
		inst:     inst,
		engine:   engine,
		account:  account,
		position: position,
		store:    store,
		mirror:   NewMirror(engine, 1001),
		events:   events,
		// orders:  make(map[int64]trading.Order),
		clients: make(map[string]int64),
	}
}

// EventPublisher is the real-time seam used by a symbol market. Publishing must
// not wait for subscriber network I/O.
type EventPublisher interface {
	Publish(stream.Commit)
}

func (s *LocalSymbolMarket) Symbol() string {
	if s == nil || s.inst == nil {
		return ""
	}
	return s.inst.Symbol
}

func (s *LocalSymbolMarket) Place(ctx context.Context, req PlaceReq) (trading.Order, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.placeLocked(ctx, req)
}

// placeLocked places an order while the caller owns s.mu.
func (s *LocalSymbolMarket) placeLocked(ctx context.Context, req PlaceReq) (trading.Order, error) {
	// TODO: 检查
	if req.ClientOrderID == "" {
		return trading.Order{}, constant.ErrClientOrderIDRequired
	}
	if req.Symbol != s.inst.Symbol {
		return trading.Order{}, constant.ErrInstrumentMismatch
	}

	key := fmt.Sprintf("%v-%v", req.AccountID, req.ClientOrderID)
	if orderID, ok := s.clients[key]; ok {
		order := s.engine.GetOrder(orderID)
		if order == nil {
			return trading.Order{}, constant.ErrOrderNotFound
		}
		return *order, nil
	}

	open := req.OrderSide == constant.Buy && req.PositionSide == constant.Long || req.OrderSide == constant.Sell && req.PositionSide == constant.Short
	if req.Leverage <= 0 {
		return trading.Order{}, constant.ErrInvalidLeverage
	}
	if req.PositionSide != constant.Long && req.PositionSide != constant.Short {
		return trading.Order{}, constant.ErrInvalidPositionSide
	}
	s.ensureAccount(req.AccountID)

	var frozen int64
	if open {
		// 开仓
		frozen = req.Price * req.Amount / req.Leverage
		if err := s.account.Freeze(req.AccountID, s.inst.Quote, frozen); err != nil {
			return trading.Order{}, err
		}
	}

	placeCmd := &matching.PlaceOrder{
		AccountID:     req.AccountID,
		ClientOrderID: req.ClientOrderID,
		Price:         req.Price,
		Amount:        req.Amount,
		Leverage:      req.Leverage,
		PositionSide:  req.PositionSide,
		OrderSide:     req.OrderSide,
		Type:          req.Type,
	}
	result := s.engine.ApplyWithResult(placeCmd)
	order, err := s.applyPlaceEvents(req, result.Events)
	if err != nil {
		s.publishLocked(result.BookChanges, nil, s.privateEvents(result.Events), result.Events)
		if open {
			s.account.Release(req.AccountID, s.inst.Quote, frozen)
		}

		return trading.Order{}, err
	}
	s.clients[key] = order.ID
	private := s.privateEvents(result.Events)
	if order.State == constant.StateInBook {
		private = append(private, s.orderUpdateEvent(&order, latestSequence(result.Events)))
	}
	s.publishLocked(result.BookChanges, s.publicTrades(result.Events), private, result.Events)

	return order, nil
}

func (s *LocalSymbolMarket) Cancel(ctx context.Context, accountID int64, orderID int64) (trading.Order, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	order := s.engine.GetOrder(orderID)
	if order == nil {
		return trading.Order{}, constant.ErrOrderNotFound
	}
	if order.AccountID != accountID {
		return trading.Order{}, constant.ErrOrderForbidden
	}

	result := s.engine.ApplyWithResult(&matching.CancelOrder{OrderID: orderID})
	for _, event := range result.Events {
		switch e := event.(type) {
		case *matching.OrderRejected:
			return trading.Order{}, fmt.Errorf("%w: %s", constant.ErrOrderRejected, e.Reason)
		case *matching.OrderCanceled:
			if err := s.releaseCanceled(order, e.Remaining); err != nil {
				return trading.Order{}, err
			}
		}
	}

	canceled := s.engine.GetOrder(orderID)
	if canceled == nil {
		return trading.Order{}, constant.ErrOrderNotFound
	}
	s.publishLocked(result.BookChanges, nil, s.privateEvents(result.Events), result.Events)
	return *canceled, nil
}

func (s *LocalSymbolMarket) applyPlaceEvents(req PlaceReq, events []matching.Event) (trading.Order, error) {
	// var taker trading.Order
	var orderID int64
	for _, evt := range events {
		switch e := evt.(type) {
		case *matching.OrderRejected:
			return trading.Order{}, fmt.Errorf("%w: %s", constant.ErrOrderRejected, e.Reason)

		case *matching.OrderAccepted:
			// 挂单
			// taker = trading.Order{
			// 	ID:            e.OrderID,
			// 	AccountID:     req.AccountID,
			// 	ClientOrderID: req.ClientOrderID,
			// 	Symbol:        req.Symbol,
			// 	OrderSide:     req.OrderSide,
			// 	PositionSide:  req.PositionSide,
			// 	Price:         req.Price,
			// 	Amount:        req.Amount,
			// 	Type:          req.Type,
			// 	Seq:           e.Seq,
			// 	State:         constant.StateAccept,
			// }
			// s.orders[e.OrderID] = taker
			orderID = e.OrderID

		case *matching.TradeEvent:
			taker := s.engine.GetOrder(e.TakerOrderID)
			maker := s.engine.GetOrder(e.MakerOrderID)
			if taker == nil || maker == nil {
				return trading.Order{}, fmt.Errorf("taker or maker order not found")
			}
			if err := s.settle(maker, e.Price, e.Amount); err != nil {
				return trading.Order{}, err
			}
			if err := s.settle(taker, e.Price, e.Amount); err != nil {
				return trading.Order{}, err
			}

		case *matching.OrderCanceled:
			order := s.engine.GetOrder(e.OrderID)
			if order == nil {
				return trading.Order{}, constant.ErrOrderNotFound
			}
			if err := s.releaseCanceled(order, e.Remaining); err != nil {
				return trading.Order{}, err
			}
		}
	}

	order := s.engine.GetOrder(orderID)
	if order == nil {
		return trading.Order{}, constant.ErrOrderNotFound
	}
	return *order, nil
}

// settle 结算成交，订单+成交价+数量
func (s *LocalSymbolMarket) settle(order *trading.Order, price int64, amount int64) error {
	// Mirror orders are synthetic liquidity and do not settle into customer accounts.
	if order.AccountID == s.mirror.accountID {
		return nil
	}

	res, err := s.position.Place(
		order.AccountID,
		order.Symbol,
		order.OrderSide,
		order.PositionSide,
		price,
		amount,
		order.Leverage,
		0,
	)
	if err != nil {
		return err
	}

	if res.Effect == constant.EffectOpen {
		frozenChange := -order.Price * amount / order.Leverage
		availableChange := order.Price*amount/order.Leverage - price*amount/order.Leverage
		return s.account.Settle(
			order.AccountID,
			s.inst.Quote,
			frozenChange,
			availableChange,
		)
	}

	return s.account.Settle(
		order.AccountID,
		s.inst.Quote,
		0,
		res.ClosedMargin+res.RealizedPnL,
	)
}

func (s *LocalSymbolMarket) releaseCanceled(order *trading.Order, remaining int64) error {
	if !isOpenOrder(order) || remaining <= 0 {
		return nil
	}

	frozen := order.Price * remaining / order.Leverage
	return s.account.Settle(
		order.AccountID,
		s.inst.Quote,
		-frozen,
		frozen,
	)
}

func (s *LocalSymbolMarket) Book(ctx context.Context) (marketdata.Book, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.bookSnapshotLocked(), nil
}

func (s *LocalSymbolMarket) ApplyBook(ctx context.Context, book marketdata.Book) error {
	if s == nil || s.engine == nil {
		return fmt.Errorf("symbol market is not initialized")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	result := s.mirror.ApplyWithResult(book)
	s.lastBook = book
	s.publishLocked(result.BookChanges, nil, nil, result.Events)
	return nil
}

func (s *LocalSymbolMarket) bookSnapshotLocked() marketdata.Book {
	const maxDepth = 1000
	bids := s.engine.OrderBook().Bids(maxDepth)
	asks := s.engine.OrderBook().Asks(maxDepth)
	book := marketdata.Book{
		Symbol:       s.symbol,
		Bids:         make([]marketdata.Level, 0, len(bids)),
		Asks:         make([]marketdata.Level, 0, len(asks)),
		LastUpdateID: s.bookSeq,
		Timestamp:    time.Now().UnixMilli(),
	}
	for _, level := range bids {
		book.Bids = append(book.Bids, marketdata.Level{
			Price:      level.Price,
			Amount:     level.Amount,
			OrderCount: level.OrderCount,
		})
	}
	for _, level := range asks {
		book.Asks = append(book.Asks, marketdata.Level{
			Price:      level.Price,
			Amount:     level.Amount,
			OrderCount: level.OrderCount,
		})
	}
	return book
}

func (s *LocalSymbolMarket) publishLocked(
	changes orderbook.ChangeSet,
	trades []stream.Trade,
	private []stream.PrivateEvent,
	events []matching.Event,
) {
	if s.events == nil || changes.Empty() && len(trades) == 0 && len(private) == 0 {
		return
	}
	commit := stream.Commit{
		Symbol:     s.symbol,
		CommandSeq: latestSequence(events),
		Timestamp:  time.Now().UnixMilli(),
		Bids:       bookLevels(changes.Bids),
		Asks:       bookLevels(changes.Asks),
		Trades:     trades,
		Private:    private,
	}
	if !changes.Empty() {
		s.bookSeq++
		commit.BookSequence = s.bookSeq
	}
	s.events.Publish(commit)
}

func (s *LocalSymbolMarket) publicTrades(events []matching.Event) []stream.Trade {
	trades := make([]stream.Trade, 0)
	for _, event := range events {
		trade, ok := event.(*matching.TradeEvent)
		if !ok {
			continue
		}
		trades = append(trades, stream.Trade{
			TradeID:   trade.TradeID,
			Price:     trade.Price,
			Amount:    trade.Amount,
			TakerSide: trade.TakerSide,
		})
	}
	return trades
}

func (s *LocalSymbolMarket) privateEvents(events []matching.Event) []stream.PrivateEvent {
	private := make([]stream.PrivateEvent, 0)
	for _, event := range events {
		switch e := event.(type) {
		case *matching.TradeEvent:
			maker := s.engine.GetOrder(e.MakerOrderID)
			taker := s.engine.GetOrder(e.TakerOrderID)
			private = s.appendTradePrivate(private, maker, taker, e)

		case *matching.OrderCanceled:
			order := s.engine.GetOrder(e.OrderID)
			if order != nil {
				private = append(private, s.orderUpdateEvent(order, e.Seq))
			}

		case *matching.OrderRejected:
			if e.AccountID <= 0 || e.AccountID == s.mirror.accountID {
				continue
			}
			private = append(private, stream.PrivateEvent{
				Kind: stream.PrivateOrderUpdate,
				Order: &stream.OrderUpdate{
					AccountID:     e.AccountID,
					OrderID:       e.OrderID,
					ClientOrderID: e.ClientOrderID,
					Symbol:        s.symbol,
					State:         constant.StateReject,
					Sequence:      e.Seq,
				},
			})
		}
	}
	return private
}

func (s *LocalSymbolMarket) appendTradePrivate(
	private []stream.PrivateEvent,
	maker *trading.Order,
	taker *trading.Order,
	trade *matching.TradeEvent,
) []stream.PrivateEvent {
	if maker != nil && maker.AccountID != s.mirror.accountID {
		makerUpdate := orderWithTradeState(maker, trade.MakerState, trade.MakerRemaining)
		private = append(private, s.orderUpdateEvent(makerUpdate, trade.Seq))
		private = append(private, stream.PrivateEvent{
			Kind: stream.PrivateFill,
			Fill: &stream.Fill{
				AccountID:     maker.AccountID,
				OrderID:       maker.ID,
				ClientOrderID: maker.ClientOrderID,
				TradeID:       trade.TradeID,
				Symbol:        maker.Symbol,
				OrderSide:     maker.OrderSide,
				PositionSide:  maker.PositionSide,
				Role:          constant.Maker,
				Price:         trade.Price,
				Amount:        trade.Amount,
			},
		})
	}
	if taker != nil && taker.AccountID != s.mirror.accountID {
		takerUpdate := orderWithTradeState(taker, trade.TakerState, trade.TakerRemaining)
		private = append(private, s.orderUpdateEvent(takerUpdate, trade.Seq))
		private = append(private, stream.PrivateEvent{
			Kind: stream.PrivateFill,
			Fill: &stream.Fill{
				AccountID:     taker.AccountID,
				OrderID:       taker.ID,
				ClientOrderID: taker.ClientOrderID,
				TradeID:       trade.TradeID,
				Symbol:        taker.Symbol,
				OrderSide:     taker.OrderSide,
				PositionSide:  taker.PositionSide,
				Role:          constant.Taker,
				Price:         trade.Price,
				Amount:        trade.Amount,
			},
		})
	}
	return private
}

func orderWithTradeState(
	order *trading.Order,
	state constant.State,
	remaining int64,
) *trading.Order {
	update := *order
	update.State = state
	update.Remaining = remaining
	return &update
}

func (s *LocalSymbolMarket) orderUpdateEvent(order *trading.Order, sequence int64) stream.PrivateEvent {
	return stream.PrivateEvent{
		Kind: stream.PrivateOrderUpdate,
		Order: &stream.OrderUpdate{
			AccountID:     order.AccountID,
			OrderID:       order.ID,
			ClientOrderID: order.ClientOrderID,
			Symbol:        order.Symbol,
			OrderSide:     order.OrderSide,
			PositionSide:  order.PositionSide,
			Type:          order.Type,
			Price:         order.Price,
			Amount:        order.Amount,
			Remaining:     order.Remaining,
			Leverage:      order.Leverage,
			State:         order.State,
			Sequence:      sequence,
		},
	}
}

func bookLevels(levels []orderbook.Level) []stream.BookLevel {
	result := make([]stream.BookLevel, 0, len(levels))
	for _, level := range levels {
		result = append(result, stream.BookLevel{
			Price:      level.Price,
			Amount:     level.Amount,
			OrderCount: level.OrderCount,
		})
	}
	return result
}

func latestSequence(events []matching.Event) int64 {
	var sequence int64
	for _, event := range events {
		if event.Sequence() > sequence {
			sequence = event.Sequence()
		}
	}
	return sequence
}

func (s *LocalSymbolMarket) Start(ctx context.Context) error {
	if s == nil {
		return fmt.Errorf("symbol market is nil")
	}
	out := make(chan L2Book, 1)
	go s.startFeed(ctx, out)
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case book, ok := <-out:
				if !ok {
					return
				}
				_ = s.ApplyBook(ctx, book)
			}
		}
	}()
	return nil
}

func (s *LocalSymbolMarket) Close(ctx context.Context) error {
	return nil
}

func (s *LocalSymbolMarket) GetOrder(ctx context.Context, accountID int64, orderID int64) (trading.Order, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	order := s.engine.GetOrder(orderID)
	if order == nil {
		return trading.Order{}, constant.ErrOrderNotFound
	}
	if order.AccountID != accountID {
		return trading.Order{}, constant.ErrOrderForbidden
	}
	return *order, nil
}

func (s *LocalSymbolMarket) Balance(ctx context.Context, accountID int64) entity.Balance {
	s.mu.Lock()
	defer s.mu.Unlock()
	balance, _ := s.account.Balance(accountID, s.inst.Quote)
	return balance
}

func (s *LocalSymbolMarket) Position(ctx context.Context, accountID int64, symbol string, positionSide constant.PositionSide) (trading.Position, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	position := s.store.GetPosition(accountID, symbol, positionSide)
	if position == nil {
		return trading.Position{}, false
	}
	return *position, true
}

// Positions returns all long and short position snapshots for an account in
// this symbol market.
func (s *LocalSymbolMarket) Positions(ctx context.Context, accountID int64) []trading.Position {
	s.mu.Lock()
	defer s.mu.Unlock()

	positions := make([]trading.Position, 0, 2)
	for _, positionSide := range []constant.PositionSide{constant.Long, constant.Short} {
		position := s.store.GetPosition(accountID, s.symbol, positionSide)
		if position != nil && position.Amount > 0 {
			positions = append(positions, *position)
		}
	}
	return positions
}

func (s *LocalSymbolMarket) ensureAccount(accountID int64) {
	if accountID <= 0 {
		return
	}
	if s.store.GetAccount(accountID) == nil {
		s.store.SaveAccount(&entity.Account{ID: accountID})
	}
}

func isOpenOrder(order *trading.Order) bool {
	return order.OrderSide == constant.Buy && order.PositionSide == constant.Long ||
		order.OrderSide == constant.Sell && order.PositionSide == constant.Short
}
