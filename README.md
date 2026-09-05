# BuaaEnergyMailMonitor

从北航能耗公开页面 `http://shsd.buaa.edu.cn/PubBuaa?id=<id>` 读取各监控点数值，低于阈值时通过 SMTP（SSL 465）发送告警邮件的命令行小工具。

- 纯 Go 标准库实现，无任何第三方依赖（HTML 解析为自写扫描器，发信用 `net/smtp` + TLS）
- 命令行以 `(id 名称 阈值)` 三元组方式指定多个监控项，逐个抓取、比较
- `-to` 参数置于命令末尾，可一次指定多个收件人分别发送；不加 `-to` 时使用 `config.go` 中的 `defaultTo`
- 低于阈值的项汇总为一封邮件发送；邮件正文由单个 `fmt.Sprintf` 生成，方便定制
- 源码不含任何真实凭据与真实监控点 id，统一使用尖括号占位符 / 占位 id，避免误提交泄露

## 目录结构

| 文件 | 职责 |
|---|---|
| `main.go` | 主逻辑：循环抓取 → 比较 → 准备邮件内容，内容非空则追加“请勿回复”声明并发送 |
| `http.go` | HTTP 抓取与 HTML 解析（定位 `svg#canvas1` 内 `<tspan>` 数值） |
| `mail.go` | 邮件正文生成（单个 `fmt.Sprintf`）与 SMTP 发送功能 |
| `config.go` | 发信邮箱配置（服务器、端口、用户名、密码、收件人） |
| `Makefile` | 编译/运行/格式化/检查/清理 |
| `LICENSE` | MIT License |

## 配置（重要）

编译前请编辑 `config.go`，把尖括号占位符替换为真实配置（占位符表示此处必须配置，禁止把真实值提交到仓库）：

```go
smtpServer = "<smtp.example.com>"  // 例: "smtp.qiye.aliyun.com"
smtpPort   = "<465>"               // SSL(隐式 TLS) 通常 465
smtpUser   = "<username>"          // 发信邮箱用户名
smtpPass   = "<password>"          // 发信邮箱密码/授权码
defaultTo  = "<recipient@example.com>" // 默认收件人，可用命令末尾的 -to 覆盖（可多个）
```

若仍为占位符，程序会在需要发信时报错并拒绝发送。  

发送邮件的内容是可修改的，若某个电表小于所设置阈值，默认内容是  
```
电表 %s（%s）还剩 %g 度，低于阈值 %g 度，请及时充值电费。
```

这部分可以在 `mail.go` 中修改。  

## 编译与运行

```bash
make              # 编译出 ./buaaenergy（等价 go build -o buaaenergy .）
make run ARGS="00001 一号电表 50 00002 二号电表 40 -to alert@example.com ops@example.com"
# 或直接:
./buaaenergy 00001 一号电表 50 00002 二号电表 40 -to alert@example.com ops@example.com
```

参数必须是 3 的倍数：每个监控项为 `(id, 名称, 阈值)` 三元组；名称含空格请加引号。
`-to` 放在命令**末尾**，其后可跟多个邮箱地址，一次向多个用户发送；不加 `-to` 时发给
`config.go` 中的 `defaultTo`。示例中的 `00001`/`00002` 为占位 id，请替换为真实监控点 id。

输出示例（示意）：

```
[00001] 一号电表: 数值=47(47) 阈值=50 -> 低于阈值，告警
[00002] 二号电表: 数值=37(37) 阈值=40 -> 低于阈值，告警
已向 2 个收件人发送告警邮件（2 项低于阈值）：alert@example.com, ops@example.com
```

其他目标：`make fmt`（格式化）、`make vet`（静态检查）、`make clean`（清理产物）。

## 许可

[MIT License](LICENSE) © 2026 Lycarus
