# internal/types — 交易领域类型

**路径：** `internal/types/`  
**职责：** 订单、合约、方向、订单类型、TIF、订单状态等撮合/账务共享模型。  
**不负责：** 行情 Tick、持久化、网络协议。

## 核心类型

### Side

```go
type Side uint8 // Buy=0, Sell=1
```

- `String()` / `Opposite()` 便于日志与撮合取对手方。

### OrderType

| 值 | 含义 |
|----|------|
| `Limit` | 限价 |
| `Market` | 市价 |

### TIF（Time In Force）

| 值 | 含义 |
|----|------|
| `GTC` | 成交前一直有效 |
| `IOC` | 立即成交剩余撤销 |
| `FOK` | 全部成交或全部撤销 |

### Order

| 字段 | 说明 |
|------|------|
| `ID` | 订单 ID（由 matching 分配） |
| `AccountID` | 账户 |
| `Side` / `Type` / `TIF` | 业务属性 |
| `Price` / `Amount` / `Remaining` | 价格、委托量、剩余量（uint64） |
| `Seq` | 全局时序号（matching 分配） |
| `State` | 见 `state.go` |

### Instrument

当前为简版：

```go
type Instrument struct {
    Symbol string // eg BTC/USDT / 期货合约
    Base   string
    Quote  string
}
```

> 对齐 PROJECT.md 期货化时，需扩展乘数、保证金率、到期日等（见 `docs/project/PROJECT.md`）。

### State（`state.go`）

`Create → Pending/Reject → Accept → InBook → Partial/Filled/Canceled/Expired`

`String()` 输出稳定字符串，供事件与日志使用。

## 依赖关系

- 被 `orderbook`、`matching`、`account` 依赖。
- 与 `internal/md.Tick` **不共享**：行情是外部参考价，订单是本所事实。
