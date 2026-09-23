# internal/account — 账户与资金

**路径：** `internal/account/account.go`  
**职责：** 账户余额（可用/冻结）的 GORM 模型与资金操作。  
**不负责：** 撮合、订单簿、行情。

## 模型

```go
type Account struct {
    ID        uint
    UserID    string  // 用户 / 子账户标识
    Currency  string
    Available int64   // 可用
    Frozen    int64   // 冻结
    CreatedAt / UpdatedAt time.Time
}
```

金额使用 `int64` 最小货币单位（或合约价值整数），避免浮点误差。

## 资金操作

| 方法 | 语义 | 实现状态 |
|------|------|----------|
| `Deposit` | 入金：增加 Available | **占位**（空实现） |
| `Withdraw` | 出金：减少 Available | **占位** |
| `Freeze` | 下单冻结：Available → Frozen | 已实现（条件 UPDATE） |
| 解冻/扣减 | 成交扣 Frozen、撤单解冻 | 需补齐（见源码后半） |

`Freeze` 使用原子条件更新：

```sql
UPDATE accounts
SET available = available - $amount,
    frozen    = frozen + $amount
WHERE user_id = ? AND currency = ? AND available >= $amount
```

并发安全依赖数据库行锁与条件更新，不依赖应用层先查后写。

## 与其它模块

- 依赖：`gorm.io/gorm`、`internal/config.InitDB` 或 `internal/global.GetDB`。
- 将来：期货保证金、盯市、持仓表应对齐 PROJECT.md M2。

## 待办

- [ ] 实现 Deposit / Withdraw
- [ ] 成交后 `Freeze → Deduct`、撤单 `Unfreeze`
- [ ] 子账户隔离与规则表乘数/保证金率
