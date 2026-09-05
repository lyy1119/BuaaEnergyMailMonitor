// config.go —— 告警邮件的配置项（SMTP 服务器、端口、用户名、密码等）。
//
// 安全约定：提交到仓库的源码一律不填写真实值，统一保留尖括号占位符，
// 形如 <smtp.example.com>、<username>、<password>，一眼即可看出需要配置；
// 使用者编译前把尖括号及其中内容整体替换为真实配置即可。
// 例：smtpServer = "smtp.qiye.aliyun.com"、smtpPort = "465"、
//
//	smtpUser = "你的发信邮箱"、smtpPass = "你的密码/授权码"。
package main

const (
	// SMTP 服务器地址（占位符示例：<smtp.example.com>）
	smtpServer = "<smtp.example.com>"

	// SMTP 端口：SSL（隐式 TLS）一般为 465，STARTTLS 一般为 587，明文为 25。
	// 替换时保留纯数字，如 "465"。
	smtpPort = "<465>"

	// 发信邮箱用户名/账号（占位符示例：<username>）
	smtpUser = "<username>"

	// 发信邮箱密码/授权码（占位符示例：<password>）
	smtpPass = "<password>"

	// 发件人地址：通常与用户名相同，可单独修改
	smtpFrom = "<username>"

	// 默认收件人地址（可用命令行 -to 参数覆盖）
	defaultTo = "<recipient@example.com>"
)
