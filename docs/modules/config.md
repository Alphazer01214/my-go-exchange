# internal/config — 配置与数据库

**路径：** `internal/config/config.go`  
**职责：** PostgreSQL 连接初始化（GORM）。  
**不负责：** `cmd/md-ingest` 的 YAML 读取（那是进程内 viper，见 [cmd-md-ingest.md](./cmd-md-ingest.md)）。

## PostgreSQL 配置结构

```go
type PostgreSQL struct {
    Host, User, Password, DBName, SSLMode string
    Port int
}
```

## InitDB

1. 校验 `Password` 非空（约定从 `PG_PASSWORD` 等环境注入，不硬编码）。
2. 拼 DSN，`gorm.Open(postgres...)`，`SkipDefaultTransaction`（热点路径手动控事务）。
3. 连接池：`MaxIdle=10`，`MaxOpen=100`，`ConnMaxLifetime=1h`，`ConnMaxIdleTime=30m`。
4. AutoMigrate 当前注释掉，生产建议显式迁移脚本。

## 使用示例

```go
db, err := config.InitDB(config.PostgreSQL{
    Host: "localhost", Port: 5432,
    User: "app", Password: os.Getenv("PG_PASSWORD"),
    DBName: "my_go_exchange", SSLMode: "disable",
})
```

也可把 `*gorm.DB` 放入 `internal/global` 供 `account` 等使用。

## 相关配置文件

`config/config.yaml` 含 `postgresql` 段与 `md-ingest` 行情段；**两套配置目前并行**，行情进程用 viper 读 YAML，DB 段需应用侧组装后调用 `InitDB`。
