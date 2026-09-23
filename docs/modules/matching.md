# internal/matching — 撮合引擎

**路径：** `internal/matching/`  
**文件：** `engine.go` / `commands.go` / `events.go`  
**职责：** 接收 Command（下单/撤单），在 orderbook 上撮合，产出 Event。  
**不负责：** 账户资金、网络接入、行情抓取。

## 模型

```text
Command ──► Engine.Apply ──► []Event
                │
                ▼
           OrderBook
```

### Command（commands.go）

| 类型 | 说明 |
|------|------|
| `PlaceOrder` | 委托：Side/Type/Price/Amount/TIF 等 |
| `CancelOrder` | 撤单 |

### Event（events.go）

| 类型 | 说明 |
|------|------|
| `OrderAccepted` / `OrderRejected` | 受理 / 拒绝 |
| `OrderCanceled` | 撤销成功 |
| `Trade` | 成交（含价格、数量、双方订单） |

事件支持 JSON 编解码，便于后续走 WS / JetStream。

## Engine

```go
type Engine struct {
    inst *types.Instrument
    ob   *orderbook.OrderBook
    seq, orderID, tradeID uint64
}
```

- **`Seq` / `OrderID` / `TradeID` 只在 Engine 分配**，orderbook 不改写。
- `Apply(cmd)` 分发 `applyPlace` / `applyCancel`。
- TIF 语义：
  - **GTC**：不能全成则剩余挂簿（`InBook`）
  - **IOC**：能成多少成多少，剩余撤销
  - **FOK**：不能全额成交则整单拒绝

## 撮合流程（Place）

1. `seq++`，基础/业务校验（失败则 `OrderRejected`）
2. 分配 `OrderID`
3. 与对手簿交叉：按价格优先、时间优先吃单，产出 `Trade`
4. 按 TIF 处理剩余量（挂簿 / 撤销 / FOK 失败回滚语义）

## 状态流转

```text
Create → Accept → InBook ⇄ Partial → Filled
                      ↘ Canceled / Expired / Reject
```

（状态常量定义在 `types.State`）

## 测试

`engine_test.go` 覆盖限价撮合、撤单、TIF 等。运行：

```bash
go test ./internal/matching/
```

## 扩展点（对接 PROJECT.md）

- 报价 bot 将作为特殊 Order 注入本引擎。
- 期货开平、保证金校验应挂在 Account + RuleConfig，不进 Engine 热路径。
