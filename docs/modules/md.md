# internal/md — 行情模型与抽象

**路径：** `internal/md/`  
**文件：** `tick.go` / `source.go`  
**职责：** 规范化行情 Tick、桥接生命周期 Status，以及 Source / Publisher 接口。  
**不负责：** 具体抓取、消息队列实现、UI。

## Tick

字段语义对齐 `ctp-crawler` 的 tick JSON，便于对照与回放。

| 字段 | 说明 |
|------|------|
| `InstrumentID` | 合约代码，如 `cu2611` |
| `Exchange` | 交易所（桥侧按品种前缀推断） |
| `Price` / `Bid` / `Ask` | 最新价、买一、卖一（`*float64`，可空） |
| `Volume` / `BidVolume` / `AskVolume` | 成交量、买卖一挂量 |
| `OpenInterest` / `Turnover` | 持仓量、成交额 |
| `Open/High/Low/PreClose/PreSettlement` | 日内高低开、昨收昨结 |
| `UpperLimit` / `LowerLimit` | 涨跌停 |
| `UpdateTime` / `UpdateMillisec` | 交易所时间 |
| `TradingDay` / `ActionDay` | 交易日 / 业务日 |
| `Timestamp` | 本地接收 unix 秒 |
| `Source` / `SourceMode` | 来源标记（如 `ctp-bridge` / `ctp`） |

价格指针型：CTP 空价（DBL_MAX 量级）在桥侧会变成 `null`，前端显示 `—`。

## Status

桥进程生命周期事件：`front_connected`、`login_ok`、`subscribe_ok`、错误等，供日志与排障。

## 接口

```go
type Source interface {
    Run(ctx context.Context, out chan<- Tick) error
    Name() string
}

type Publisher interface {
    Publish(ctx context.Context, tick Tick) error
    Close() error
    Name() string
}
```

| 实现 | 包 | 说明 |
|------|-----|------|
| CTP 子进程桥 | `internal/md/bridge` | 当前默认 Source |
| JetStream | `internal/md/natspub` | 当前默认 Publisher |

后续 HTTP 快照轮询可再实现 Source，上层管道不用改。

## JSON 约定

- Tick 序列化为单行 JSON，经 stdout（桥）或 JetStream subject `md.tick.<instrument>` 传递。
- 额外字段可加，但不要改已有字段名，保证前后端兼容。
