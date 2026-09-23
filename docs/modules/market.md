# internal/market — 行情热缓存（QuoteHub）

**路径：** `internal/market/hub.go`  
**职责：** 进程内缓存最新盘口、1 分钟 K 线、逐笔 tape，并向 WebSocket 订阅者扇出事件。  
**不负责：** 持久化、CTP 接入、HTTP 协议细节。

> 这是 UI 用的**有界内存缓存**，不是 Redis/DB。进程退出即清；持久化在 JetStream。

## 数据结构

```text
Hub
├── mu   sync.RWMutex
├── inst map[string]*instrumentState
├── subs map[chan Event]struct{}
└── total int64

instrumentState
├── last md.Tick        // 最新一笔
├── bars []Bar          // 1m K 线，≤240
└── tape []TapeItem     // 最近逐笔，≤200
```

| 常量 | 值 | 含义 |
|------|-----|------|
| `tapeLimit` | 200 | 逐笔保留条数 |
| `barLimit` | 240 | K 线根数（约 4 小时 1m） |
| `subBufSize` | 256 | 每订阅者事件缓冲 |

## 写路径：OnTick

1. 写锁
2. 懒创建 `instrumentState`
3. 覆盖 `last`
4. `tape` 追加，超限截断（保留末尾）
5. `upsertBar`：按分钟对齐
   - 新分钟 → 新 bar（OHLCV）
   - 同分钟 → 更新 H/L/C/V
6. 解锁后 `broadcast(tick / kline)`

K 线用 `time.Now()` 对齐分钟；Volume 采用 tick 累计成交量快照口径。

## 读路径

| 方法 | 用途 | 特点 |
|------|------|------|
| `Snapshot()` | `/api/quotes`、WS 首帧 | 拷贝 bars/tape，避免读写竞争 |
| `Quote(id)` | 单合约最新 | 读锁 |
| `Klines(id)` | 单合约 K 线 | 读锁并拷贝 |
| `Subscribe()` | WS 扇出 | 返回 channel + unsub 闭包 |

慢订阅者：`broadcast` 非阻塞发送，缓冲满则丢弃该客户端事件（保写入吞吐）。

## Event

```json
{"type":"tick","data":{ ...Tick }}
{"type":"kline","data":{"instrument_id":"cu2611","bar":{...}}}
{"type":"snapshot","data":{"quotes":[],"tape":[],"klines":{}}}
```

## 与 JetStream 的分工

| | Hub | JetStream `MD_TICKS` |
|--|-----|----------------------|
| 用途 | UI 热读 | 持久化 / 回放 / 多消费者 |
| 容量 | 有界小历史 | 按 Retention 配置 |
| 重启 | 丢失 | 可恢复 |

## 扩展

- 多周期 K 线：可在 `instrumentState` 增加 5m/15m bar 切片或从 1m 聚合。
- 分布式：多实例时改为从 NATS 订阅重建 Hub，或下推到前端订阅 stream。
