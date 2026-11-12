package account

import (
	"errors"
	"fmt"
	"my-go-exchange/internal/entity"
	"sort"
	"sync"
)

var (
	ErrInsufficientBalance       = errors.New("insufficient balance")
	ErrInsufficientFrozenBalance = errors.New("insufficient frozen balance")
)

type Ledger struct {
	mu       sync.Mutex
	balances map[string]entity.Balance // "account_id-currency" -> balance
}

func NewLedger() *Ledger {
	return &Ledger{
		balances: make(map[string]entity.Balance),
	}
}

func (l *Ledger) Deposit(accountID int64, currency string, amount int64) error {
	l.mu.Lock()
	defer l.mu.Unlock()

	key := fmt.Sprintf("%v-%v", accountID, currency)
	b := l.balances[key]
	b.Available += amount
	l.balances[key] = b
	return nil
}

func (l *Ledger) Withdraw(accountID int64, currency string, amount int64) error {
	l.mu.Lock()
	defer l.mu.Unlock()

	key := fmt.Sprintf("%v-%v", accountID, currency)
	b := l.balances[key]
	if b.Available < amount {
		return ErrInsufficientBalance
	}
	b.Available -= amount
	l.balances[key] = b
	return nil
}

func (l *Ledger) Freeze(accountID int64, currency string, amount int64) error {
	l.mu.Lock()
	defer l.mu.Unlock()

	key := fmt.Sprintf("%v-%v", accountID, currency)
	b := l.balances[key]
	if b.Available < amount {
		return ErrInsufficientBalance
	}
	b.Available -= amount
	b.Frozen += amount
	l.balances[key] = b
	return nil
}

func (l *Ledger) Release(accountID int64, currency string, amount int64) error {
	l.mu.Lock()
	defer l.mu.Unlock()

	key := fmt.Sprintf("%v-%v", accountID, currency)
	b := l.balances[key]
	if b.Frozen < amount {
		return ErrInsufficientFrozenBalance
	}
	b.Frozen -= amount
	b.Available += amount
	l.balances[key] = b
	return nil
}

func (l *Ledger) Balance(accountID int64, currency string) (entity.Balance, bool) {
	l.mu.Lock()
	defer l.mu.Unlock()
	b, ok := l.balances[l.key(accountID, currency)]
	return b, ok
}

// Balances returns a deterministic snapshot of all currencies owned by an
// account.
func (l *Ledger) Balances(accountID int64) []entity.Balance {
	l.mu.Lock()
	defer l.mu.Unlock()

	balances := make([]entity.Balance, 0)
	for _, balance := range l.balances {
		if balance.AccountID == accountID {
			balances = append(balances, balance)
		}
	}
	sort.Slice(balances, func(i, j int) bool {
		return balances[i].Currency < balances[j].Currency
	})
	return balances
}

func (l *Ledger) Settle(accountID int64, currency string, frozenChange int64, abailableChange int64) error {
	l.mu.Lock()
	defer l.mu.Unlock()

	key := l.key(accountID, currency)
	b := l.balances[key]

	frozen := b.Frozen + frozenChange
	available := b.Available + abailableChange
	if frozen < 0 {
		return ErrInsufficientFrozenBalance
	}
	if available < 0 {
		return ErrInsufficientBalance
	}

	b.AccountID = accountID
	b.Currency = currency
	b.Frozen = frozen
	b.Available = available
	l.balances[key] = b
	return nil
}

func (l *Ledger) key(accountID int64, currency string) string {
	return fmt.Sprintf("%v-%v", accountID, currency)
}
