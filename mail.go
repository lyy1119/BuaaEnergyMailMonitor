// mail.go —— 告警邮件的全部功能：
// 邮件正文生成（单个 fmt.Sprintf，便于使用者定制）与 SMTP 发送
// （SSL 465，LOGIN/PLAIN 认证，均用 Go 标准库 net/smtp 实现）。
// 邮件相关配置见 config.go（真实账号密码不写入仓库，保留尖括号占位符）。
package main

import (
	"crypto/tls"
	"encoding/base64"
	"fmt"
	"io"
	"net"
	"net/smtp"
	"os"
	"strings"
	"time"
)

// alertItemContent 生成单个“低于阈值”监控项的邮件内容。
// 输入参数依次为：id、name、threshold、current。
// 整段内容只用一个 fmt.Sprintf 生成，需要定制邮件文案时改这一行即可。
func alertItemContent(id, name string, threshold, current float64) string {
	return fmt.Sprintf("电表 %s（%s）还剩 %g 度，低于阈值 %g 度，请及时充值电费。\n", name, id, current, threshold)
}

// sendAlertMail 向多个收件人发送告警邮件：正文 body 由主逻辑（main.go）拼装，
// 末尾已含“请勿回复”声明；subject 为邮件主题（由主逻辑指定）；
// lowCount 为低于阈值的项数（保留参数，便于调用方扩展主题/正文时使用）。
// 每个收件人单独发送一封邮件，一个收件人失败不影响其他收件人；
// 全部失败时返回错误，部分失败时在 stderr 打印失败明细并返回 nil。
func sendAlertMail(tos []string, subject string, body string) error {
	// 收件人预处理：去空、去占位符地址
	var targets []string
	for _, t := range tos {
		t = strings.TrimSpace(t)
		if t == "" || strings.ContainsAny(t, "<>") {
			continue
		}
		targets = append(targets, t)
	}
	if len(targets) == 0 {
		return fmt.Errorf("没有有效的收件人地址：请用 -to 指定（可多个），或修改 config.go 中的 defaultTo")
	}
	// 防呆：若仍为尖括号占位符，说明使用者尚未替换 config.go，拒绝发信
	for _, cfg := range []struct{ name, val string }{
		{"smtpServer", smtpServer},
		{"smtpPort", smtpPort},
		{"smtpUser", smtpUser},
		{"smtpPass", smtpPass},
		{"smtpFrom", smtpFrom},
	} {
		if strings.ContainsAny(cfg.val, "<>") {
			return fmt.Errorf("config.go 中 %s 仍为占位符 %q：请替换为真实值（服务器/端口/邮箱用户名/密码/发件人）后重新编译", cfg.name, cfg.val)
		}
	}

	addr := net.JoinHostPort(smtpServer, smtpPort)
	auths, err := smtpAuths(addr, smtpServer, smtpUser, smtpPass)
	if err != nil {
		return fmt.Errorf("SMTP 探测失败: %w", err)
	}

	sent := 0
	var failures []string
	for _, to := range targets {
		msg := buildMessage(smtpFrom, to, subject, body)
		var lastErr error
		ok := false
		for _, a := range auths {
			if err := sendOnce(addr, smtpServer, smtpFrom, to, msg, a); err == nil {
				ok = true
				break
			} else {
				lastErr = err
			}
		}
		if ok {
			sent++
		} else {
			failures = append(failures, fmt.Sprintf("%s: %v", to, lastErr))
		}
	}

	if sent == 0 {
		return fmt.Errorf("所有收件人发送均失败: %s", strings.Join(failures, "；"))
	}
	if len(failures) > 0 {
		fmt.Fprintf(os.Stderr, "部分收件人发送失败（成功 %d/%d）: %s\n", sent, len(targets), strings.Join(failures, "；"))
	}
	return nil
}

// buildMessage 组装符合 RFC5322 的邮件原文（CRLF 换行，中文主题用 RFC2047 编码）。
func buildMessage(from, to, subject, body string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "From: %s\r\n", from)
	fmt.Fprintf(&b, "To: %s\r\n", to)
	fmt.Fprintf(&b, "Subject: %s\r\n", encodeRFC2047(subject))
	fmt.Fprintf(&b, "Date: %s\r\n", time.Now().Format(time.RFC1123Z))
	fmt.Fprintf(&b, "MIME-Version: 1.0\r\n")
	fmt.Fprintf(&b, "Content-Type: text/plain; charset=UTF-8\r\n")
	fmt.Fprintf(&b, "Content-Transfer-Encoding: 8bit\r\n")
	b.WriteString("\r\n")
	b.WriteString(strings.ReplaceAll(body, "\n", "\r\n"))
	return b.String()
}

func encodeRFC2047(s string) string {
	return "=?UTF-8?B?" + base64.StdEncoding.EncodeToString([]byte(s)) + "?="
}

// smtpAuths 连接服务器并返回服务器声明的、程序支持的认证方式
// （优先 LOGIN，其次 PLAIN，均为阿里企业邮箱常见方式）。
func smtpAuths(addr, host, user, pass string) ([]smtp.Auth, error) {
	conn, err := tlsDial(addr, host)
	if err != nil {
		return nil, err
	}
	defer conn.Close()
	c, err := smtp.NewClient(conn, host)
	if err != nil {
		return nil, err
	}
	defer c.Close()
	ok, ext := c.Extension("AUTH") // 触发 EHLO
	if !ok {
		return nil, fmt.Errorf("服务器不支持 SMTP AUTH")
	}
	ext = strings.ToUpper(ext)
	var auths []smtp.Auth
	if strings.Contains(ext, "LOGIN") {
		auths = append(auths, &loginAuth{username: user, password: pass})
	}
	if strings.Contains(ext, "PLAIN") {
		auths = append(auths, smtp.PlainAuth("", user, pass, host))
	}
	if len(auths) == 0 {
		return nil, fmt.Errorf("服务器声明的 AUTH 方式(%q)不支持", ext)
	}
	return auths, nil
}

// sendOnce 用给定认证方式完成一次发送。
func sendOnce(addr, host, from, to, msg string, auth smtp.Auth) error {
	conn, err := tlsDial(addr, host)
	if err != nil {
		return err
	}
	c, err := smtp.NewClient(conn, host)
	if err != nil {
		conn.Close()
		return err
	}
	defer c.Close()
	if err := c.Auth(auth); err != nil {
		return fmt.Errorf("认证失败: %w", err)
	}
	if err := c.Mail(from); err != nil {
		return err
	}
	if err := c.Rcpt(to); err != nil {
		return err
	}
	w, err := c.Data()
	if err != nil {
		return err
	}
	if _, err := io.WriteString(w, msg); err != nil {
		w.Close()
		return err
	}
	if err := w.Close(); err != nil {
		return err
	}
	return c.Quit()
}

// tlsDial 建立到 SMTP 的隐式 TLS(SSL) 连接。
func tlsDial(addr, serverName string) (net.Conn, error) {
	d := net.Dialer{Timeout: 15 * time.Second}
	raw, err := d.Dial("tcp", addr)
	if err != nil {
		return nil, err
	}
	conn := tls.Client(raw, &tls.Config{ServerName: serverName, MinVersion: tls.VersionTLS12})
	if err := conn.SetDeadline(time.Now().Add(30 * time.Second)); err != nil {
		raw.Close()
		return nil, err
	}
	if err := conn.Handshake(); err != nil {
		raw.Close()
		return nil, err
	}
	conn.SetDeadline(time.Time{})
	return conn, nil
}

// loginAuth 实现 SMTP AUTH LOGIN（net/smtp 未内置该机制）。
type loginAuth struct {
	username, password string
}

func (a *loginAuth) Start(*smtp.ServerInfo) (string, []byte, error) {
	return "LOGIN", nil, nil
}

func (a *loginAuth) Next(fromServer []byte, more bool) ([]byte, error) {
	if !more {
		return nil, nil
	}
	prompt := strings.ToLower(strings.TrimSpace(string(fromServer)))
	switch {
	case strings.Contains(prompt, "user"):
		return []byte(a.username), nil
	case strings.Contains(prompt, "pass"):
		return []byte(a.password), nil
	default:
		return nil, fmt.Errorf("未知的 AUTH LOGIN 提示: %q", fromServer)
	}
}
