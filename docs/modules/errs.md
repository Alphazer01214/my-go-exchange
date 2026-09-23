# internal/errs — 订单错误定义

**路径：** `internal/errs/order.go`  
**职责：** 下单/撤单校验用的哨兵错误，供 `orderbook.Validate*` 与 `matching` 统一引用。  
**不负责：** HTTP 状态码映射、错误码表（见 `docs/err.md`）。

## 错误分组

### 基础参数校验（Place 最先）

| 错误 | 含义 |
|------|------|
| `ErrInvalidSide` | 买卖方向非法 |
| `ErrInvalidOrderType` | 订单类型非法 |
| `ErrInvalidTIF` | TIF 非法 |
| `ErrInvalidPrice` | 价格必须 > 0 |
| `ErrInvalidAmount` | 数量必须 > 0 |

### 业务规则校验（Place 第二步）

| 错误 | 含义 |
|------|------|
| `ErrMarketOrderWithPrice` | 市价单不得带价格 |
| `ErrMarketOrderCannotRest` | 市价单不能挂簿 |
| `ErrDuplicateOrderID` | 订单 ID 重复 |
| `ErrFOKCannotFill` | FOK 无法全部成交 |

### 撤单校验

| 错误 | 含义 |
|------|------|
| `ErrOrderNotFound` | 订单不存在 |
| `ErrOrderNotCancellable` | 当前状态不可撤 |

## 使用约定

- 用 `errors.Is` 比较，不要比字符串。
- 业务层可 `fmt.Errorf("%w: ...", errs.ErrXxx)` 包装细节，保留可判定性。
