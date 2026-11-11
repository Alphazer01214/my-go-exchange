package position

import "my-go-exchange/internal/store"

type Side string

const (
	Long  Side = "long"
	Short      = "short"
)

type Position struct {
}

type Manager struct {
	store *store.Store
}

// Open 开仓
type Open struct {
	Position    store.Position
	RealizedPnL int64
	ClosedSize  int64 // 平仓数量

}

func NewManager(store *store.Store) *Manager {
	return &Manager{store}
}

// Open 开仓
func (m *Manager) Open(accountID int64, symbol string, side string, posSide string, price int64, amount int64, leverage int64, fee int64) (*Open, error) {

}
