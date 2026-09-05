// main.go —— 只保留主逻辑：
//
//	在循环中依次：抓取页面数值 -> 与阈值比较 -> 低于阈值则准备（累积）邮件内容；
//	循环结束后，若邮件内容非空（存在低于阈值的项），在内容末尾追加“请勿回复”
//	声明，然后发送邮件。
//
// HTTP 相关功能见 http.go；邮件配置见 config.go；邮件功能见 mail.go。
package main

import (
	"flag"
	"fmt"
	"os"
	"strconv"
	"strings"
)

func usage() {
	fmt.Fprintf(os.Stderr, `用法: buaaenergy [-to 收件人邮箱] id1 名称1 阈值1 [id2 名称2 阈值2 ...]

依次抓取 http://shsd.buaa.edu.cn/PubBuaa?id=<id> 页面中 svg#canvas1 里 <tspan>
的数值，若低于对应阈值则汇总发送告警邮件。

发信配置见 config.go：编译前请把其中的占位符（形如 <smtp.example.com>、
<username>、<password>）替换为真实配置，仓库中不包含任何真实账号密码。
收件人默认取 config.go 中的 defaultTo，也可用 -to 参数覆盖。

参数必须是 3 的倍数：每个监控项为 (id, 名称, 阈值) 三元组，阈值支持小数。
名称含空格时请用引号包裹。

示例:
  buaaenergy -to alert@example.com 00001 一号电表 50 00002 二号电表 40
`)
}

func main() {
	to := flag.String("to", defaultTo, "收件人邮箱地址（默认取 config.go 中的 defaultTo）")
	flag.Usage = usage
	flag.Parse()

	args := flag.Args()
	if len(args) == 0 || len(args)%3 != 0 {
		usage()
		os.Exit(2)
	}

	var body strings.Builder // 邮件正文：在循环中准备，循环结束后按需发送
	lowCount := 0            // 低于阈值的项数
	for i := 0; i+2 < len(args); i += 3 {
		id, name, thStr := args[i], args[i+1], args[i+2]
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
	if err := sendAlertMail(*to, lowCount, body.String()); err != nil {
		fmt.Fprintf(os.Stderr, "发送告警邮件失败: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("已向 %s 发送告警邮件（%d 项低于阈值）。\n", *to, lowCount)
}
