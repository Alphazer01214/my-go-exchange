# internal/global — 全局句柄

**路径：** `internal/global/global.go`  
**职责：** 进程级共享的 `*gorm.DB`。

```go
var DB *gorm.DB

func GetDB() *gorm.DB { return DB }
```

## 说明

- 简单全局变量风格，供 `account` 等直接取 DB。
- 初始化应在进程启动时（`config.InitDB` 后）写入 `global.DB`。
- **不建议**在库代码里 new DB；测试可注入 stub 或真实容器 DB。

## 替代方案（后续可演进）

显式依赖传递（构造函数注入 `*gorm.DB`）优于全局变量；保留 global 是为了少改现有骨架。
