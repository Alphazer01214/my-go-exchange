# internal/md/bridge + tools/md_bridge — CTP 行情桥

**路径：**  
- Go：`internal/md/bridge/source.go`  
- Python：`tools/md_bridge/md_bridge.py`

**职责：** 用 Python 官方 CTPAPI 绑定连交易所前置，把行情 NDJSON 交给 Go 管道。  
**不负责：** JetStream 发布、Web UI、撮合。

## 为什么混入 Python

纯 Go 直连 CTP 线协议需逆向加密握手（CFTDC），短期不可靠。故 **仅协议面用 Python + `openctp-ctp`**，其余（规范化、缓存、MQ、Web）全部 Go。

## 数据流

```mermaid
flowchart LR
  F["CTP 前置<br/>SimNow/TTS"] --> P["md_bridge.py<br/>SubscribeMarketData"]
  P -->|"stdout NDJSON"| G["bridge.Source"]
  G -->|"chan md.Tick"| C["md-ingest 管道"]
```

### Python 输出协议（stdout）

| `type` | 载荷 |
|--------|------|
| `tick` | 见 [md.md](./md.md) Tick |
| `status` | 登录/订阅/断线等事件 |

### Go 侧

`bridge.Source.Run`：

1. `exec.CommandContext` 启动 `python md_bridge.py --front ... --symbols ...`
2. 扫描 stdout，按 `type` 反序列化为 `md.Tick` / `md.Status`
3. Tick 写入 `out` channel；Status 打日志

## Python 桥要点

- 依赖：`pip install openctp-ctp`
- 默认前置：SimNow `tcp://182.254.243.31:40011`（Broker `9999`）
- openctp TTS `tcp://trading.openctp.cn:30011` 需 TTS 专用绑定，标准 `openctp-ctp` 会断线 `4097`
- 合约前缀推断交易所（SHFE/DCE/CZCE/CFFEX/INE/GFEX）
- CTP 空价（极大 double）转为 `null`

## 配置（config.yaml → bridge）

```yaml
bridge:
  python: python
  script: tools/md_bridge/md_bridge.py
  front: "tcp://182.254.243.31:40011"
  broker: "9999"
  symbols: [cu2611, al2611, rb2701, au2612]
```

## 手动调试

```bash
python tools/md_bridge/md_bridge.py --front tcp://182.254.243.31:40011 --symbols cu2611,al2611
```

## 依赖与边界

- 实现 `md.Source`；上层只见 `chan md.Tick`。
- 进程模型：Go 父进程托管 Python 子进程，ctx 取消时随管道结束。
