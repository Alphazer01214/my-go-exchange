package store

import (
	"fmt"
	"my-go-exchange/internal/trading"
	"sync"

	"my-go-exchange/internal/constant"
	"my-go-exchange/internal/entity"
)

type Store struct {
	mu sync.RWMutex

	inst     map[string]*trading.Instrument // BTCUSDT -> inst
	account  map[int64]*entity.Account
	position map[string]*trading.Position // "account_id-symbol-position_side" -> position
}

func NewStore() *Store {
	return &Store{
		inst:     make(map[string]*trading.Instrument),
		account:  make(map[int64]*entity.Account),
		position: make(map[string]*trading.Position),
	}
}

func (s *Store) GetInst(symbol string) *trading.Instrument {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.inst[symbol]
}

func (s *Store) GetAccount(id int64) *entity.Account {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.account[id]
}

func (s *Store) GetPosition(accountID int64, symbol string, positionSide constant.PositionSide) *trading.Position {
	s.mu.RLock()
	defer s.mu.RUnlock()
	key := fmt.Sprintf("%v-%v-%v", accountID, symbol, positionSide)
	return s.position[key]
}

// ListPositionsBySymbol returns value snapshots so callers can scan without
// retaining pointers into the store after its lock is released.
func (s *Store) ListPositionsBySymbol(symbol string) []trading.Position {
	s.mu.RLock()
	defer s.mu.RUnlock()

	positions := make([]trading.Position, 0)
	for _, position := range s.position {
		if position.Symbol == symbol && position.Amount > 0 {
			positions = append(positions, *position)
		}
	}
	return positions
}

func (s *Store) SavePosition(position *trading.Position) {
	s.mu.Lock()
	defer s.mu.Unlock()
	key := fmt.Sprintf("%v-%v-%v", position.AccountID, position.Symbol, position.PositionSide)
	s.position[key] = position
}

func (s *Store) DeletePosition(accountID int64, symbol string, positionSide constant.PositionSide) {
	s.mu.Lock()
	defer s.mu.Unlock()
	key := fmt.Sprintf("%v-%v-%v", accountID, symbol, positionSide)
	delete(s.position, key)
}

func (s *Store) SaveAccount(account *entity.Account) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.account[account.ID] = account
}

func (s *Store) SaveInst(inst *trading.Instrument) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.inst[inst.Symbol] = inst
}
