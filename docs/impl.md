
## Orderbook

### Level
level: 例如：买1买2...卖1卖2...，由 某价位+订单量组成
作为行情数据，不含明细

### entry
作为功能的实现

### order
作为事实数据，包含订单全部信息

**seq** 作为有顺序的序列，存在于所有带有时序特征的结构体。

### 下单/撤单


### sidebook
包括 ask 和 bid 两个方向的订单


## Matching

### command
定义外部可以提交给引擎的命令：下单、撤单，以及 STP 模式
order
- place
- cancel

### event
定义引擎产出的事件：接受、拒绝、撤销、成交，以及 JSON 编解码
order
- cancel
- reject
- place

trade

### matching
实现撮合

