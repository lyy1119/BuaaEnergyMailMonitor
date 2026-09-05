// main.go —— 只保留主逻辑：
//
//	在循环中依次：抓取页面数值 -> 与阈值比较 -> 低于阈值则准备（累积）邮件内容；
//	循环结束后，若邮件内容非空（存在低于阈值的项），在内容末尾追加“请勿回复”
//	声明，然后发送邮件（可一次发给多个收件人）。
//
// HTTP 相关功能见 http.go；邮件配置见 config.go；邮件功能见 mail.go。
package main

import (
	"fmt"
	"os"
	"strconv"
	"strings"
)

func usage() {
	fmt.Fprintf(os.Stderr, `用法: buaaenergy id1 名称1 阈值1 [id2 名称2 阈值2 ...] [-to 收件人1 [收件人2 ...]]

依次抓取 http://shsd.buaa.edu.cn/PubBuaa?id=<id> 页面中 svg#canvas1 里 <tspan>
的数值，若低于对应阈值则汇总发送告警邮件。

发信配置见 config.go：编译前请把其中的占位符（形如 <smtp.example.com>、
<username>、<password>）替换为真实配置，仓库中不包含任何真实账号密码。

收件人：
  - -to 放在命令末尾，其后可跟一个或多个邮箱地址，一次向多个用户发送；
  - 不加 -to 时，使用 config.go 中的 defaultTo（无需修改 defaultTo 也可用 -to 覆盖）。

参数必须是 3 的倍数：每个监控项为 (id, 名称, 阈值) 三元组，阈值支持小数。
名称含空格时请用引号包裹。

示例:
  buaaenergy 00001 一号电表 50 00002 二号电表 40 -to a@x.com b@y.com c@z.com
`)
}

// validEmail 做简单的收件人地址合法性检查（非空、含 @、@ 后有域名点、无空白/尖括号/逗号）。
func validEmail(s string) bool {
	if s == "" || strings.ContainsAny(s, " <>,") {
		return false
	}
	at := strings.IndexByte(s, '@')
	if at <= 0 || at == len(s)-1 {
		return false
	}
	return strings.IndexByte(s[at+1:], '.') > 0
}

func main() {
	args := os.Args[1:]

	// 解析命令行：三元组在前；-to 分隔符放末尾，其后的每个参数都是一个收件人
	var triples []string
	var recipients []string
	usedTo := false
	for i, a := range args {
		if a == "-to" {
			triples = args[:i]
			recipients = args[i+1:]
			usedTo = true
			break
		}
	}
	if !usedTo {
		triples = args
		// 未提供 -to：使用 config.go 中的 defaultTo
		recipients = []string{defaultTo}
	}
	if len(triples) == 0 || len(triples)%3 != 0 {
		usage()
		os.Exit(2)
	}
	if usedTo && len(recipients) == 0 {
		fmt.Fprintln(os.Stderr, "错误：-to 后至少需要提供一个收件人邮箱地址")
		usage()
		os.Exit(2)
	}

	// 收件人过滤：去掉无效地址与重复项
	var tos []string
	seen := make(map[string]bool)
	for _, r := range recipients {
		r = strings.TrimSpace(r)
		if !validEmail(r) {
			if usedTo {
				fmt.Fprintf(os.Stderr, "跳过无效收件人地址: %q\n", r)
			}
			continue
		}
		if seen[r] {
			continue
		}
		seen[r] = true
		tos = append(tos, r)
	}
	if len(tos) == 0 {
		if !usedTo {
			fmt.Fprintf(os.Stderr, "错误：未指定收件人，且 config.go 中 defaultTo（%q）仍是无效/占位地址：请用 -to 指定收件人，或修改 config.go 的 defaultTo\n", defaultTo)
		} else {
			fmt.Fprintln(os.Stderr, "错误：没有有效的收件人邮箱地址")
		}
		os.Exit(2)
	}

	var body strings.Builder // 邮件正文：在循环中准备，循环结束后按需发送
	lowCount := 0            // 低于阈值的项数
	for i := 0; i+2 < len(triples); i += 3 {
		id, name, thStr := triples[i], triples[i+1], triples[i+2]
		th, err := strconv.ParseFloat(thStr, 64)
		if err != nil {
			fmt.Fprintf(os.Stderr, "[%s/%s] 阈值 %q 不是有效数字\n", id, name, thStr)
			os.Exit(2)
		}

		// 1) 抓取并解析当前值（http.go）
		current, raw, err := fetchCanvas1Value(id)
		if err != nil {
			fmt.Fprintf(os.Stderr, "[%s/%s] 获取失败: %v\n", id, name, err)
			continue
		}

		// 2) 与阈值比较
		if current < th {
			// 3) 低于阈值：生成并累积邮件内容（mail.go 中的单个 fmt.Sprintf）
			lowCount++
			body.WriteString(alertItemContent(id, name, th, current))
			fmt.Printf("[%s] %s: 数值=%s(%g) 阈值=%g -> 低于阈值，告警\n", id, name, raw, current, th)
		} else {
			fmt.Printf("[%s] %s: 数值=%s(%g) 阈值=%g -> 正常\n", id, name, raw, current, th)
		}
	}

	// 循环结束：邮件内容为空 -> 无低于阈值的项，不发送
	if body.Len() == 0 {
		fmt.Println("本次运行没有低于阈值的项目，未发送邮件。")
		return
	}

	// 确认需要发送后，在末尾追加“请勿回复”声明，再发送邮件
	body.WriteString("\n——————————————\n本邮件由 BUAA 能耗监控程序自动发送，请勿直接回复。\n")
	if err := sendAlertMail(tos, "电表电量过低提醒", lowCount, body.String()); err != nil {
		fmt.Fprintf(os.Stderr, "发送告警邮件失败: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("已向 %d 个收件人发送告警邮件（%d 项低于阈值）：%s\n", len(tos), lowCount, strings.Join(tos, ", "))
}
