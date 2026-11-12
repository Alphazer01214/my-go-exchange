package market

import (
	"context"
	"errors"
	"sort"
	"sync"
	"time"

	"my-go-exchange/internal/account"
	"my-go-exchange/internal/constant"
	"my-go-exchange/internal/entity"
	"my-go-exchange/internal/marketdata"
	"my-go-exchange/internal/matching"
	"my-go-exchange/internal/portfolio"
	"my-go-exchange/internal/store"
	"my-go-exchange/internal/stream"
	"my-go-exchange/internal/trading"
)

// Market 大市场，装载所有品种交易平台并共享账户与持仓。
type Market struct {
	mu sync.RWMutex

	account  *account.Ledger
	position *portfolio.Manager
	store    *store.Store
	ids      *matching.IDAllocator
	feeds    map[string]MarketSource
	markets  map[string]SymbolMarket // symbol -> SymbolMarket
	streams  *stream.Hub
}

func NewMarket() *Market {
	marketStore := store.NewStore()
	return &Market{
		account:  account.NewLedger(),
		position: portfolio.NewManager(marketStore),
		store:    marketStore,
		ids:      matching.NewIDAllocator(),
		feeds:    make(map[string]MarketSource),
		markets:  make(map[string]SymbolMarket),
		streams:  stream.NewHub(stream.Config{}),
	}
}

// RegisterLocal 装载一个由本地 matching engine 驱动的品种市场。
func (m *Market) RegisterLocal(inst *trading.Instrument) (*LocalSymbolMarket, error) {
	if inst == nil {
		return nil, errors.New("instrument is required")
	}

	m.mu.Lock()
	defer m.mu.Unlock()
	if _, exists := m.markets[inst.Symbol]; exists {
		return nil, errors.New("symbol market already registered")
	}

	m.store.SaveInst(inst)
	local := NewLocalSymbolMarket(inst, m.account, m.position, m.store, m.ids, m.streams)
	m.markets[inst.Symbol] = local
	return local, nil
}

// Streams exposes the real-time read model used by WebSocket and snapshot
// adapters. The hub is independent from the matching engine.
func (m *Market) Streams() *stream.Hub {
	if m == nil {
		return nil
	}
	return m.streams
}

// RegisterSymbolMarket 装载外部或本地的 SymbolMarket 实现。
func (m *Market) RegisterSymbolMarket(symbolMarket SymbolMarket) error {
	if symbolMarket == nil || symbolMarket.Symbol() == "" {
		return errors.New("symbol market and symbol are required")
	}

	m.mu.Lock()
	defer m.mu.Unlock()
	if _, exists := m.markets[symbolMarket.Symbol()]; exists {
		return errors.New("symbol market already registered")
	}
	m.markets[symbolMarket.Symbol()] = symbolMarket
	return nil
}

// RegisterSource 注册某个品种的行情源。
func (m *Market) RegisterSource(symbol string, source MarketSource) error {
	if symbol == "" || source == nil {
		return errors.New("symbol and source are required")
	}

	m.mu.Lock()
	defer m.mu.Unlock()
	m.feeds[symbol] = source
	return nil
}

func (m *Market) Deposit(ctx context.Context, accountID int64, currency string, amount int64) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.ensureAccount(accountID)
	return m.account.Deposit(accountID, currency, amount)
}

func (m *Market) Place(ctx context.Context, req PlaceReq) (trading.Order, error) {
	symbolMarket, err := m.symbolMarket(req.Symbol)
	if err != nil {
		return trading.Order{}, err
	}
	return symbolMarket.Place(ctx, req)
}

func (m *Market) Cancel(ctx context.Context, accountID int64, orderID int64) (trading.Order, error) {
	m.mu.RLock()
	markets := make([]SymbolMarket, 0, len(m.markets))
	for _, symbolMarket := range m.markets {
		markets = append(markets, symbolMarket)
	}
	m.mu.RUnlock()

	var lastErr error
	for _, symbolMarket := range markets {
		order, err := symbolMarket.Cancel(ctx, accountID, orderID)
		if err == nil {
			return order, nil
		}
		if !errors.Is(err, constant.ErrOrderNotFound) {
			return trading.Order{}, err
		}
		lastErr = err
	}
	if lastErr != nil {
		return trading.Order{}, lastErr
	}
	return trading.Order{}, constant.ErrOrderNotFound
}

func (m *Market) GetOrder(ctx context.Context, accountID int64, orderID int64) (trading.Order, error) {
	m.mu.RLock()
	markets := make([]SymbolMarket, 0, len(m.markets))
	for _, symbolMarket := range m.markets {
		markets = append(markets, symbolMarket)
	}
	m.mu.RUnlock()

	var lastErr error
	for _, symbolMarket := range markets {
		order, err := symbolMarket.GetOrder(ctx, accountID, orderID)
		if err == nil {
			return order, nil
		}
		if !errors.Is(err, constant.ErrOrderNotFound) {
			return trading.Order{}, err
		}
		lastErr = err
	}
	if lastErr != nil {
		return trading.Order{}, lastErr
	}
	return trading.Order{}, constant.ErrOrderNotFound
}

func (m *Market) Balance(ctx context.Context, accountID int64, currency string) entity.Balance {
	m.mu.RLock()
	defer m.mu.RUnlock()
	balance, _ := m.account.Balance(accountID, currency)
	return balance
}

// Balances returns every currency balance for an account.
func (m *Market) Balances(ctx context.Context, accountID int64) []entity.Balance {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.account.Balances(accountID)
}

func (m *Market) Position(
	ctx context.Context,
	accountID int64,
	symbol string,
	positionSide constant.PositionSide,
) (trading.Position, bool) {
	m.mu.RLock()
	symbolMarket := m.markets[symbol]
	m.mu.RUnlock()

	if local, ok := symbolMarket.(*LocalSymbolMarket); ok {
		return local.Position(ctx, accountID, symbol, positionSide)
	}
	return trading.Position{}, false
}

func (m *Market) Book(ctx context.Context, symbol string) (marketdata.Book, error) {
	symbolMarket, err := m.symbolMarket(symbol)
	if err != nil {
		return marketdata.Book{}, err
	}
	return symbolMarket.Book(ctx)
}

// Instruments returns a deterministic snapshot of registered instruments.
func (m *Market) Instruments() []trading.Instrument {
	m.mu.RLock()
	defer m.mu.RUnlock()

	instruments := make([]trading.Instrument, 0, len(m.markets))
	for symbol, symbolMarket := range m.markets {
		instrument := m.store.GetInst(symbol)
		if instrument != nil {
			instruments = append(instruments, *instrument)
			continue
		}
		if market := symbolMarket; market != nil && market.Symbol() != "" {
			instruments = append(instruments, trading.Instrument{Symbol: market.Symbol()})
		}
	}
	sort.Slice(instruments, func(i, j int) bool {
		return instruments[i].Symbol < instruments[j].Symbol
	})
	return instruments
}

// Positions returns all position snapshots for an account across local symbol
// markets.
func (m *Market) Positions(ctx context.Context, accountID int64) []trading.Position {
	m.mu.RLock()
	markets := make([]*LocalSymbolMarket, 0, len(m.markets))
	for _, symbolMarket := range m.markets {
		if local, ok := symbolMarket.(*LocalSymbolMarket); ok {
			markets = append(markets, local)
		}
	}
	m.mu.RUnlock()

	positions := make([]trading.Position, 0)
	for _, symbolMarket := range markets {
		positions = append(positions, symbolMarket.Positions(ctx, accountID)...)
	}
	sort.Slice(positions, func(i, j int) bool {
		if positions[i].Symbol == positions[j].Symbol {
			return positions[i].PositionSide < positions[j].PositionSide
		}
		return positions[i].Symbol < positions[j].Symbol
	})
	return positions
}

func (m *Market) Start(ctx context.Context) error {
	_ = m.ScanLiquidations(ctx)
	go m.runLiquidationScanner(ctx)

	m.mu.RLock()
	markets := make(map[string]SymbolMarket, len(m.markets))
	for symbol, symbolMarket := range m.markets {
		markets[symbol] = symbolMarket
	}
	feeds := make(map[string]MarketSource, len(m.feeds))
	for symbol, source := range m.feeds {
		feeds[symbol] = source
	}
	m.mu.RUnlock()

	for symbol, symbolMarket := range markets {
		source, ok := feeds[symbol]
		if !ok {
			if err := symbolMarket.Start(ctx); err != nil {
				return err
			}
			continue
		}

		out := make(chan L2Book, 1)
		go func(source MarketSource, out chan<- L2Book) {
			_ = source.Run(ctx, out)
		}(source, out)
		go func(symbolMarket SymbolMarket, out <-chan L2Book) {
			for {
				select {
				case <-ctx.Done():
					return
				case book, ok := <-out:
					if !ok {
						return
					}
					_ = symbolMarket.ApplyBook(ctx, book)
				}
			}
		}(symbolMarket, out)
	}
	return nil
}

// LiquidationScanInterval is the interval between liquidation scans.
const LiquidationScanInterval = 500 * time.Millisecond

func (m *Market) runLiquidationScanner(ctx context.Context) {
	ticker := time.NewTicker(LiquidationScanInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			_ = m.ScanLiquidations(ctx)
		}
	}
}

// ScanLiquidations scans every locally matched symbol once. It is exported for
// deterministic tests and operational diagnostics; Start runs it every
// LiquidationScanInterval.
func (m *Market) ScanLiquidations(ctx context.Context) error {
	m.mu.RLock()
	markets := make([]*LocalSymbolMarket, 0, len(m.markets))
	for _, symbolMarket := range m.markets {
		if local, ok := symbolMarket.(*LocalSymbolMarket); ok {
			markets = append(markets, local)
		}
	}
	m.mu.RUnlock()

	var errs []error
	for _, symbolMarket := range markets {
		if err := symbolMarket.ScanLiquidations(ctx); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

func (m *Market) Close(ctx context.Context) error {
	m.mu.RLock()
	markets := make([]SymbolMarket, 0, len(m.markets))
	for _, symbolMarket := range m.markets {
		markets = append(markets, symbolMarket)
	}
	m.mu.RUnlock()

	for _, symbolMarket := range markets {
		if err := symbolMarket.Close(ctx); err != nil {
			return err
		}
	}
	return nil
}

func (m *Market) symbolMarket(symbol string) (SymbolMarket, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	symbolMarket, ok := m.markets[symbol]
	if !ok {
		return nil, constant.ErrInstrumentMismatch
	}
	return symbolMarket, nil
}

func (m *Market) ensureAccount(accountID int64) {
	if accountID <= 0 {
		return
	}
	if m.store.GetAccount(accountID) == nil {
		m.store.SaveAccount(&entity.Account{ID: accountID})
	}
}
