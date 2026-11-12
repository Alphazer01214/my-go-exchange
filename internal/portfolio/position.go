package portfolio

import (
	"errors"
	"my-go-exchange/internal/trading"

	"my-go-exchange/internal/constant"
	"my-go-exchange/internal/store"
)

// type Manager struct {
// }

type Manager struct {
	store *store.Store
}

func liquidationPrice(entryPrice int64, margin int64, amount int64, positionSide constant.PositionSide) (int64, error) {
	if amount <= 0 {
		return 0, errors.New("position amount must be positive")
	}

	switch positionSide {
	case constant.Long:
		return entryPrice - margin/amount, nil
	case constant.Short:
		return entryPrice + margin/amount, nil
	default:
		return 0, errors.New("invalid position side")
	}
}

func realizedPnL(entryPrice int64, price int64, amount int64, positionSide constant.PositionSide) (int64, error) {
	switch positionSide {
	case constant.Long:
		return (price - entryPrice) * amount, nil
	case constant.Short:
		return (entryPrice - price) * amount, nil
	default:
		return 0, errors.New("invalid position side")
	}
}

// Open 开仓
type OpenResult struct {
	Position     trading.Position
	RealizedPnL  int64
	ClosedSize   int64 // 平仓数量
	ClosedMargin int64 // 本次平仓释放的保证金
	Effect       constant.PositionEffect
}

func NewManager(store *store.Store) *Manager {
	return &Manager{store}
}

func (m *Manager) Place(accountID int64, symbol string, orderSide constant.OrderSide, positionSide constant.PositionSide, price int64, amount int64, leverage int64, fee int64) (*OpenResult, error) {
	// 开多/空
	if orderSide == constant.Sell && positionSide == constant.Short || orderSide == constant.Buy && positionSide == constant.Long {
		return m.Open(accountID, symbol, positionSide, price, amount, leverage, fee)
	}

	// 平多/空
	return m.Close(accountID, symbol, positionSide, price, amount, fee)
}

// Open 开仓
func (m *Manager) Open(accountID int64, symbol string, positionSide constant.PositionSide, price int64, amount int64, leverage int64, fee int64) (*OpenResult, error) {
	if amount <= 0 {
		return nil, errors.New("position amount must be positive")
	}
	if leverage <= 0 {
		return nil, errors.New("leverage must be positive")
	}

	inst := m.store.GetInst(symbol)
	if inst == nil {
		return nil, errors.New("instrument not found")
	}

	account := m.store.GetAccount(accountID)
	if account == nil {
		return nil, errors.New("account not found")
	}

	position := m.store.GetPosition(accountID, symbol, positionSide)

	if position != nil {
		// 已有持仓，加仓
		previousAmount := position.Amount
		position.EntryPrice = (position.EntryPrice*previousAmount + price*amount) / (previousAmount + amount)
		position.Amount += amount
		position.Leverage = leverage
		position.Margin += price * amount / leverage
		position.RealizedPnL -= fee
		liqPrice, err := liquidationPrice(position.EntryPrice, position.Margin, position.Amount, position.PositionSide)
		if err != nil {
			return nil, err
		}
		position.LiqPrice = liqPrice
		m.store.SavePosition(position)
		return &OpenResult{
			Position:    *position,
			Effect:      constant.EffectOpen,
			RealizedPnL: -fee,
		}, nil
	}

	margin := price * amount / leverage
	liqPrice, err := liquidationPrice(price, margin, amount, positionSide)
	if err != nil {
		return nil, err
	}
	newPosition := &trading.Position{
		AccountID:    accountID,
		Symbol:       symbol,
		PositionSide: positionSide,
		Amount:       amount,
		EntryPrice:   price,
		Leverage:     leverage,
		Margin:       margin,
		RealizedPnL:  -fee,
		LiqPrice:     liqPrice,
	}
	m.store.SavePosition(newPosition)
	return &OpenResult{
		Position:    *newPosition,
		RealizedPnL: -fee,
		Effect:      constant.EffectOpen,
	}, nil
}

// Close 平仓
func (m *Manager) Close(accountID int64, symbol string, positionSide constant.PositionSide, price int64, amount int64, fee int64) (*OpenResult, error) {
	if amount <= 0 {
		return nil, errors.New("close amount must be positive")
	}
	if price <= 0 {
		return nil, errors.New("close price must be positive")
	}

	position := m.store.GetPosition(accountID, symbol, positionSide)
	if position == nil {
		return nil, errors.New("position not found")
	}
	if position.Amount <= 0 {
		return nil, errors.New("position amount must be positive")
	}
	if amount > position.Amount {
		return nil, errors.New("close amount exceeds position")
	}

	closedMargin := position.Margin * amount / position.Amount
	pnl, err := realizedPnL(position.EntryPrice, price, amount, positionSide)
	if err != nil {
		return nil, err
	}

	position.Amount -= amount
	position.Margin -= closedMargin
	position.RealizedPnL += pnl - fee
	position.UnrealizedPnL = 0
	position.MarkPrice = price
	position.LiqPrice = 0
	if position.Amount > 0 {
		liqPrice, err := liquidationPrice(position.EntryPrice, position.Margin, position.Amount, position.PositionSide)
		if err != nil {
			return nil, err
		}
		position.LiqPrice = liqPrice
		m.store.SavePosition(position)
	} else {
		m.store.DeletePosition(accountID, symbol, positionSide)
	}

	return &OpenResult{
		Position:     *position,
		RealizedPnL:  pnl - fee,
		ClosedSize:   amount,
		ClosedMargin: closedMargin,
		Effect:       constant.EffectClose,
	}, nil
}
