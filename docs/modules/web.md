# internal/web — 行情展示后端与前端

**路径：** `internal/web/`  
**文件：** `server.go` / `embed.go` / `static/`  
**职责：** REST + WebSocket + 内嵌行情终端页面。  
**不负责：** CTP 抓取、撮合下单、账务。

## HTTP 接口

| 方法 | 路径 | 说明 |
|------|------|------|
| GET | `/` | 行情终端 UI |
| GET | `/app.js` `/style.css` | 静态资源 |
| GET | `/api/health` | 存活与数据源信息 |
| GET | `/api/meta` | stream/subject/前置等元数据 |
| GET | `/api/quotes` | 全量最新 + tape + klines |
| GET | `/api/quotes/{id}` | 单合约最新 tick |
| GET | `/api/kline/{id}` | 单合约 1 分钟 bar |
| WS | `/ws` | 实时推送 |

## WebSocket 协议

连接后先推一帧快照，随后增量：

```json
{"type":"snapshot","data":{"quotes":[...],"tape":[...],"klines":{...},"total":N,"meta":{...}}}
{"type":"tick","data":{ /* md.Tick */ }}
{"type":"kline","data":{"instrument_id":"cu2611","bar":{ /* market.Bar */ }}}
```

- 服务端 20s Ping 保活。
- 事件来自 `market.Hub.Subscribe()`。
- 同源限制放宽（`CheckOrigin` 全通过），本地开发友好；生产应收紧。

## 前端（static/）

| 文件 | 说明 |
|------|------|
| `index.html` | 布局：顶栏 / 列表 / 详情+K线 / 逐笔 |
| `style.css` | 暗色交易终端主题 |
| `app.js` | WS 客户端、表格刷新、canvas K 线 |

交互：

- 点击合约行切换详情
- 最新价涨跌着色 + 行闪烁
- 1 分钟 K 线蜡烛图 + 最新价虚线
- 断线自动重连

静态资源用 `go:embed` 打进二进制（`embed.go` 将 `static/` 映射为根路径）。

## 依赖

- `market.Hub`：读缓存与订阅
- `github.com/gorilla/websocket`
- 不直连 NATS（保持展示层薄）

## 可观测

`/api/health` 返回 source/publisher/nats/front/symbols，便于确认管道是否热。
