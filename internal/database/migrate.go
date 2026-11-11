package database

import (
	"my-go-exchange/internal/store"

	"gorm.io/gorm"
)

func Migrate(db *gorm.DB) error {
	return db.AutoMigrate(
		&store.Account{},
		&store.Instrument{},
		&store.Position{},
		&store.Order{},
		&store.Trade{},
		&store.Fill{},
		&store.LedgerEntry{},
	)
}
