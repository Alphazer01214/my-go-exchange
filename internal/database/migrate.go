package database

import (
	"my-go-exchange/internal/entity"

	"gorm.io/gorm"
)

func Migrate(db *gorm.DB) error {
	return db.AutoMigrate(
		&entity.Account{},
		&entity.Balance{},
		&entity.Instrument{},
		&entity.Position{},
		&entity.Order{},
		&entity.Trade{},
		&entity.Fill{},
		&entity.LedgerEntry{},
	)
}
