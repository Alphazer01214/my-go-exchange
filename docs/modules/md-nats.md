# internal/md/natspub + natsserver — 行情消息总线

**路径：**  
- `internal/md/natspub/publisher.go`  
- `internal/md/natsserver/embedded.go`

**职责：** 将 Tick 写入 NATS JetStream；可选进程内嵌 broker（无需 Docker）。  
**不负责：** 行情抓取、UI。

## 为什么是 NATS JetStream

- 消息队列语义（持久化、至少一次、可回放），比裸 channel 正规
- 比 Kafka 轻，单二进制 / 可内嵌，适合本地开发
- 后续 exchange / web 可独立消费 `md.tick.>`

## Stream 约定

| 项 | 值 |
|----|-----|
| Stream | `MD_TICKS` |
| Subject | `md.tick.<instrument_id>` |
| 存储 | FileStorage，保留 24h（LimitsPolicy） |
| 载荷 | Tick 的 JSON |

## natspub.Publisher

实现 `md.Publisher`：

```go
Publish(ctx, tick) → js.Publish("md.tick."+tick.InstrumentID, json)
```

- 启动时 `ensureStream`：不存在则 `AddStream`，存在则 `UpdateStream`
- 连接生命周期由调用方（md-ingest）持有

## natsserver.Embedded

进程内 NATS + JetStream：

```go
natsserver.Start(Options{Port, StoreDir, Logger})
// ClientURL() / Shutdown()
```

- 默认 `127.0.0.1:4222`，`data/nats` 落盘
- 端口占用时回退随机端口
- 也可 `nats.embedded: false`，改连外部 `nats.url`

## 配置

```yaml
nats:
  embedded: true
  port: 4222
  store_dir: data/nats
  url: "nats://127.0.0.1:4222"
```

## 消费示例

```text
subject: md.tick.cu2611
stream:  MD_TICKS
```

验证可用临时 Go 脚本 `js.New` + `StreamInfo` / `SubscribeSync`（scratch 中有样例）。

## 边界

- Hub（[market.md](./market.md)）是 UI 热缓存；JetStream 是持久总线，历史回放应走 stream。
- 多副本 web 时，各实例应各自订阅 stream 或再加网关，而不是共享 Hub 内存。
