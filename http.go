// http.go —— 与 HTTP 抓取、HTML 解析相关的全部功能：
// 请求 http://shsd.buaa.edu.cn/PubBuaa?id=<id>，定位 <svg id="canvas1">，
// 取出其中 <tspan> 的文本并解析为数值。仅用 Go 标准库（自写扫描器）。
package main

import (
	"fmt"
	"html"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
	"unicode"
)

const baseURL = "http://shsd.buaa.edu.cn/PubBuaa"

// fetchHTML downloads the page body for the given id.
func fetchHTML(id string) (string, error) {
	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Get(baseURL + "?id=" + id)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("GET %s?id=%s returned status %s", baseURL, id, resp.Status)
	}
	b, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

// nextTag locates the next '<' after from, returns the full '<...>' tag
// (up to the matching '>', honouring quoted attribute values) and the index
// just past it. ok is false when no more tags exist.
func nextTag(s string, from int) (tag string, end int, ok bool) {
	lt := strings.IndexByte(s[from:], '<')
	if lt < 0 {
		return "", len(s), false
	}
	lt += from
	inQuote := byte(0)
	i := lt + 1
	for ; i < len(s); i++ {
		c := s[i]
		if inQuote != 0 {
			if c == inQuote {
				inQuote = 0
			}
			continue
		}
		switch c {
		case '\'', '"':
			inQuote = c
		case '>':
			return s[lt : i+1], i + 1, true
		}
	}
	return s[lt:], len(s), true
}

// svgByID returns the raw markup of the first <svg ... id="canvas1" ...>...</svg>
// element, nesting-aware with respect to inner <svg> tags.
func svgByID(s string, id string) (string, error) {
	pos := 0
	for {
		idx := strings.Index(s[pos:], id)
		if idx < 0 {
			return "", fmt.Errorf("no %q found in page", id)
		}
		attrStart := pos + idx
		// Walk backwards to the opening '<svg' of the tag that carries this id.
		tagOpen := strings.LastIndex(s[:attrStart], "<svg")
		if tagOpen < 0 {
			return "", fmt.Errorf("no <svg> tag containing %q found", id)
		}
		tag, end, _ := nextTag(s, tagOpen)
		if !strings.HasPrefix(tag, "<svg") {
			return "", fmt.Errorf("malformed svg tag near %q", id)
		}
		// Depth-first scan for the matching </svg>, counting nested <svg>.
		depth := 1
		from := end
		for depth > 0 {
			t, e, ok := nextTag(s, from)
			if !ok {
				return "", fmt.Errorf("unterminated <svg id=%q> element", id)
			}
			if strings.HasPrefix(t, "<svg") && !strings.HasPrefix(t, "</") {
				depth++
			} else if strings.HasPrefix(t, "</svg") {
				depth--
				if depth == 0 {
					return s[tagOpen:e], nil
				}
			}
			from = e
		}
		pos = end
	}
}

// tspans extracts the (entity-decoded, trimmed) text of every <tspan> element
// anywhere inside the given markup.
func tspans(s string) []string {
	var out []string
	pos := 0
	for {
		idx := strings.Index(s[pos:], "<tspan")
		if idx < 0 {
			break
		}
		start := pos + idx
		tag, afterOpen, _ := nextTag(s, start)
		if !strings.HasPrefix(tag, "<tspan") {
			pos = start + len("<tspan")
			continue
		}
		closeIdx := strings.Index(s[afterOpen:], "</tspan>")
		if closeIdx < 0 {
			break
		}
		raw := s[afterOpen : afterOpen+closeIdx]
		pos = afterOpen + closeIdx + len("</tspan>")
		var b strings.Builder
		for {
			lt := strings.IndexByte(raw, '<')
			if lt < 0 {
				b.WriteString(raw)
				break
			}
			b.WriteString(raw[:lt])
			t, e, _ := nextTag(raw, lt)
			raw = raw[e:]
			if strings.HasPrefix(t, "</tspan") {
				break
			}
		}
		text := html.UnescapeString(b.String())
		text = strings.TrimFunc(strings.TrimSpace(text), unicode.IsSpace)
		out = append(out, text)
	}
	return out
}

// fetchCanvas1Value 抓取页面并返回 svg#canvas1 中第一个 tspan 的数值
// （raw 为原始文本，v 为解析后的浮点数）。
func fetchCanvas1Value(id string) (v float64, raw string, err error) {
	body, err := fetchHTML(id)
	if err != nil {
		return 0, "", err
	}
	svg, err := svgByID(body, `id="canvas1"`)
	if err != nil {
		return 0, "", fmt.Errorf("id=%s 页面解析失败: %w", id, err)
	}
	vals := tspans(svg)
	if len(vals) == 0 {
		return 0, "", fmt.Errorf("id=%s svg#canvas1 内未找到 <tspan>", id)
	}
	raw = vals[0]
	v, err = strconv.ParseFloat(strings.TrimSpace(raw), 64)
	if err != nil {
		return 0, "", fmt.Errorf("id=%s tspan 内容 %q 不是有效数字: %w", id, raw, err)
	}
	return v, raw, nil
}
