# internal/orderbook — 订单簿

**路径：** `internal/orderbook/orderbook.go`  
**职责：** 单合约限价订单簿：挂单、撤单、撮合寻价、盘口 Level 查询。  
**不负责：** 分配 OrderID/Seq（归 `matching`）、资金冻结（归 `account`）、行情 Tick。

## 概念

| 术语 | 含义 |
|------|------|
| **Level** | 某价位的汇总：`Price + Amount + OrderCount`（对外行情口径，无明细） |
| **priceLevel** | 同价位 FIFO 订单队列（`container/list`） |
| **entry** | 订单簿内部节点：`order + level + list.Element`，O(1) 定位撤单/吃单 |
| **sideBook** | 单边簿：ask 价升序 / bid 价降序的 `levels` + 价格索引 |
| **seq** | 全局时序；orderbook **不改写** Seq，只读 |

## 结构

```text
OrderBook
├── bids sideBook  (价格从高到低)
└── asks sideBook  (价格从低到高)

sideBook
├── levels []*priceLevel   // 有序档位
└── index  map[price]*level
```

- 同价 FIFO：同价位先到先撮（`orders` 链表头）。
- 档位查找：`sort.Search` 二分定位插入点。

## 主要能力

| 操作 | 行为 |
|------|------|
| 挂单 `rest` | 进入对应价位队列，累计 `totalAmount` / `orderCount` |
| 撤单 | 经 `entry` O(1) 删除并维护档位汇总 |
| 吃单 `front` | 返回对手价最优队列头订单 |
| ValidatePlace / ValidateCancel | 参数与业务规则，错误来自 `internal/errs` |

## 校验规则摘要

- 市价单：不得带价、不能挂簿（IOC/FOK 语义由 matching 处理）。
- 限价：Price > 0，Amount > 0。
- FOK：若不能全额成交则拒绝。

## 测试

`orderbook_test.go` 覆盖挂撤、价序、市价可用量等。  
> 注意：`TestAvailableForFOK` 目前在部分环境下会失败（与行情链路无关，属撮合内核既有问题）。

## 设计要点（对应 docs/impl.md）

- Level 面向行情展示；entry/order 面向功能与事实数据。
- 双边 sideBook 分离，便于 best bid/ask 与交叉判断。
