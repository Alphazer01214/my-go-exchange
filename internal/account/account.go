package account

import (
	"context"
	"errors"
	"time"

	"gorm.io/gorm"
)

// Account 账户模型
type Account struct {
	ID        uint   `gorm:"primaryKey"`
	UserID    string `gorm:"not null;size:64"`
	Currency  string `gorm:"not null;size:64"`
	Available int64  `gorm:"available"`
	Frozen    int64  `gorm:"frozen"`
	CreatedAt time.Time
	UpdatedAt time.Time
}

// Deposit 存入资金 (增加可用余额)
func Deposit(ctx context.Context, tx *gorm.DB, userID string, currency string, amount int64) error {

	return nil
}

// Withdraw 提取资金 (减少可用余额)
func Withdraw(ctx context.Context, userID string, currency string, amount int64) error {
	return nil
}

// Freeze 冻结资金 (下单时调用，从可用余额转移到冻结余额)
func Freeze(ctx context.Context, tx *gorm.DB, userID string, currency string, amount int64) error {
	res := tx.WithContext(ctx).Model(&Account{}).Where("user_id = ? and available >= ? and currency = ? ", userID, amount, currency).Updates(map[string]interface{}{
		"available": gorm.Expr("available - ?", amount),
		"frozen":    gorm.Expr("frozen + ?", amount),
	})

	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return errors.New("insufficient balance")
	}
	return nil
}

// Unfreeze 解冻资金 (取消订单时调用，从冻结余额转回可用余额)
func Unfreeze(ctx context.Context, tx *gorm.DB, userID string, currency string, amount int64) error {
	res := tx.WithContext(ctx).Model(&Account{}).Where("user_id = ? and currency = ? and frozen >= ?", userID, currency, amount).Updates(map[string]interface{}{
		"available": gorm.Expr("available + ?", amount),
		"frozen":    gorm.Expr("frozen - ?", amount),
	})

	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return errors.New("insufficient balance")
	}
	return nil
}

// Settle 处理成交
func Settle(ctx context.Context, tx *gorm.DB, userID string, currency string, amount int64) error {
	res := tx.WithContext(ctx).Model(&Account{}).Where("user_id = ? and currency = ? and frozen >= ?", userID, currency, amount).Updates(map[string]interface{}{
		"available": gorm.Expr("available - ?", amount),
		"frozen":    gorm.Expr("frozen - ?", amount),
	})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return errors.New("insufficient balance")
	}

	return nil
}
