# GSR Service 操作 MySQL 产品表

本例只演示一张 `products` 表的创建、按 ID 查询、更新和删除。`productService` 使用 `gsr.Dispatcher` 将 Command 绑定到强类型 Handler，并在自身 Handler 内执行 SQL；MainService 只能通过 Command 访问它。`*sql.DB` 是由 Go main 创建和关闭的连接池，不是另一个 Service，也不进入 GSR Core。

首次使用时，在本机 MySQL 创建**专用测试库**并执行一次建表脚本（不要指向已有业务库）；如果库和账号已经存在，跳过初始化。创建账号前，把下面的 `<强密码>` 替换成符合本机 MySQL 密码策略的密码；不要使用固定的示例口令：

```sh
mysql -u root -p -e 'CREATE DATABASE gsr_product_demo CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci'
mysql -u root -p gsr_product_demo < examples/mysql-product/schema.sql
mysql -u root -p -e "CREATE USER 'gsr_demo'@'127.0.0.1' IDENTIFIED BY '<强密码>'; GRANT SELECT, INSERT, UPDATE, DELETE ON gsr_product_demo.* TO 'gsr_demo'@'127.0.0.1'"
```

从仓库根目录运行。把连接串放进权限为 `0600` 的 `.env.local`，该文件已被 Git 忽略，不提交真实密码：

```sh
# .env.local；先把 <强密码> 换成创建账号时使用的密码。
export GSR_MYSQL_DSN='gsr_demo:<强密码>@tcp(127.0.0.1:3306)/gsr_product_demo'
```

```sh
chmod 600 .env.local
source .env.local
go run ./examples/mysql-product
# product <新 ID>: create/read/update/delete ok
mysql -u root -p gsr_product_demo -e 'SELECT COUNT(*) FROM products'
# 0：示例只删除自己刚创建的产品；AUTO_INCREMENT 计数仍可能前进
```

测试账号只用于本机演示，不要用于生产。实际部署从配置或密钥系统传入 `GSR_MYSQL_DSN`，按部署要求配置 TLS。示例对连接、读写和每次 SQL 操作设置有限等待；调用中的 Service 会等待数据库，但其它 Service 仍可运行。数据库故障返回稳定的 `product.store_unavailable`，不存在的 ID 返回 `product.not_found`，底层数据库错误仅进入服务端日志，不作为业务回复。

`schema.sql` 不自动执行或迁移；应用账号只有本表的增删改查权限。价格使用整数分，避免浮点金额误差。`UPDATE` 返回零影响行时会再检查记录是否存在，因为 MySQL 默认将“写入相同值”计为零变更行。操作超时或断线时，写入结果可能不确定，尤其不能盲目重试 `CREATE`；演示进程中途失败也可能留下它创建的产品。示例未处理跨 Service/跨进程并发写入的业务一致性，也不实现库存扣减协议。

```sh
go test ./examples/mysql-product
```

默认测试使用 SQL mock 检查 Service 的 SQL、参数和错误语义，不启动 MySQL。需要重复验证真实驱动和数据库时，先配置上述专用测试账号和 `.env.local`，再显式运行：

```sh
source .env.local
GSR_MYSQL_INTEGRATION=1 go test ./examples/mysql-product -run '^TestProductExampleLiveMySQL$' -count=1
```

集成测试会通过产品 Service 写入并删除自己创建的产品，还会验证相同值更新成功、删除后的更新返回未找到；它在执行前核对账号、地址和库名，只允许使用本机 `gsr_demo` 与 `gsr_product_demo`，避免误指向其它数据库。
