# internal/api — 交易 API（占位）

**路径：** `internal/api/`  
**文件：** `api.go`（空包声明）、`store.go`  
**职责（规划）：** 类 CTP 语义的交易接口：报单、撤单、查询、回报推送。  
**当前状态：** **未实现**，仅保留包位。

## 规划职责（PROJECT.md）

- 类 CTP WS 协议子集：报单/撤单/成交回报/查持仓资金/行情订阅。
- 调用 `internal/matching` 与 `internal/account`。
- 与 `internal/web` 的看盘 UI 解耦（交易通道可独立 WS）。

## 与 web 的边界

| | `internal/web` | `internal/api`（规划） |
|--|----------------|------------------------|
| 行情展示 | 已实现 | 可复用 market/Hub |
| 下单 | 不做 | 做 |
| 持仓/结算查询 | 只读展示 | 做 |

## 待办

- [ ] 冻结 WS 消息子集（PROJECT.md Q1）
- [ ] 实现 Place/Cancel/Query
- [ ] 对接 matching.Engine + account
