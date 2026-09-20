package xray

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"regexp"
	"strings"
	"sync"
)

var (
	accessLogRe = regexp.MustCompile(`^(\d{4}/\d{2}/\d{2} \d{2}:\d{2}:\d{2})(?:\.\d+)? (?:from )?([\d\.:]+) (?:accepted|rejected) (.*?) \[(.*?)\](?: email: (.*?))?$`)
	errorLogRe  = regexp.MustCompile(`^(\d{4}/\d{2}/\d{2} \d{2}:\d{2}:\d{2})(?:\.\d+)? \[(.*?)\] (?:\[\d+\] )?(.*?):\s*(.*)$`)

	logBufferPool = sync.Pool{
		New: func() any {
			b := make([]byte, 64*1024)
			return &b
		},
	}
)

type AccessLogEntry struct {
	Time        string `json:"time"`
	FromIP      string `json:"from_ip"`
	Protocol    string `json:"protocol"`
	Target      string `json:"target"`
	Route       string `json:"route"`
	InboundTag  string `json:"inbound_tag"`
	OutboundTag string `json:"outbound_tag"`
	Email       string `json:"email"`
	Action      string `json:"action"`
	Raw         string `json:"raw"`
}

type ErrorLogEntry struct {
	Time     string `json:"time"`
	Level    string `json:"level"`
	Module   string `json:"module"`
	Message  string `json:"message"`
	SmartTip string `json:"smartTip"`
	Raw      string `json:"raw"`
}

type LogFilter struct {
	InboundTag string
	Keyword    string
}

// ReadLastLines 从文件末尾反向读取
func ReadLastLines(filePath string, maxLines int) ([]string, error) {
	return ReadLastLinesFiltered(filePath, maxLines, LogFilter{})
}

// ParseLogLineFast 基于字节数组的高速过滤与元信息提取，零堆分配
func ParseLogLineFast(raw []byte, filter LogFilter) (matched bool, timeStr, fromIP, target string) {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 {
		return false, "", "", ""
	}

	if filter.InboundTag != "" {
		tag1 := []byte("[" + filter.InboundTag + " ->")
		tag2 := []byte("[" + filter.InboundTag + "]")
		if !bytes.Contains(trimmed, tag1) && !bytes.Contains(trimmed, tag2) {
			return false, "", "", ""
		}
	}

	if filter.Keyword != "" {
		if !bytes.Contains(bytes.ToLower(trimmed), []byte(strings.ToLower(filter.Keyword))) {
			return false, "", "", ""
		}
	}

	str := string(trimmed)
	if m := accessLogRe.FindStringSubmatch(str); len(m) >= 5 {
		return true, m[1], m[2], m[3]
	}

	return true, "", "", ""
}

// ReadLastLinesFiltered 从文件末尾反向高效分块读取（64KB Chunk Buffer，带 10MB/30,000 行安全预算与 sync.Pool 缓冲复用）
func ReadLastLinesFiltered(filePath string, maxLines int, filter LogFilter) ([]string, error) {
	if filePath == "" {
		return []string{}, nil
	}
	if maxLines <= 0 {
		maxLines = 100
	}
	if maxLines > 3000 {
		maxLines = 3000
	}

	file, err := os.Open(filePath)
	if err != nil {
		return nil, fmt.Errorf("open log file failed: %w", err)
	}
	defer file.Close()

	stat, err := file.Stat()
	if err != nil {
		return nil, err
	}
	fileSize := stat.Size()
	if fileSize == 0 {
		return []string{}, nil
	}

	const maxScanBytes = int64(10 * 1024 * 1024) // 最大逆向扫描 10MB
	const maxScanLines = 30000                   // 最大逆向扫描 30,000 行

	bufPtr := logBufferPool.Get().(*[]byte)
	defer logBufferPool.Put(bufPtr)
	buf := *bufPtr

	var lines []string
	bufSize := int64(len(buf))
	offset := fileSize
	var remainder []byte
	scannedLines := 0
	scannedBytes := int64(0)

	var inbTag1, inbTag2 []byte
	if filter.InboundTag != "" {
		inbTag1 = []byte("[" + filter.InboundTag + " ->")
		inbTag2 = []byte("[" + filter.InboundTag + "]")
	}
	lowerKw := []byte(strings.ToLower(filter.Keyword))

	matchLine := func(line []byte) bool {
		if inbTag1 != nil {
			if !bytes.Contains(line, inbTag1) && !bytes.Contains(line, inbTag2) {
				return false
			}
		}
		if len(lowerKw) > 0 {
			if !bytes.Contains(bytes.ToLower(line), lowerKw) {
				return false
			}
		}
		return true
	}

	for offset > 0 && len(lines) < maxLines && scannedLines < maxScanLines && scannedBytes < maxScanBytes {
		readSize := bufSize
		if offset < readSize {
			readSize = offset
		}
		offset -= readSize
		scannedBytes += readSize

		n, err := file.ReadAt(buf[:readSize], offset)
		if err != nil && err != io.EOF {
			return nil, err
		}
		chunk := buf[:n]

		for len(chunk) > 0 && len(lines) < maxLines && scannedLines < maxScanLines {
			idx := bytes.LastIndexByte(chunk, '\n')
			if idx == -1 {
				newRemainder := make([]byte, len(chunk)+len(remainder))
				copy(newRemainder, chunk)
				copy(newRemainder[len(chunk):], remainder)
				remainder = newRemainder
				chunk = nil
				break
			}

			linePart := chunk[idx+1:]
			var fullLine []byte
			if len(remainder) > 0 {
				fullLine = make([]byte, len(linePart)+len(remainder))
				copy(fullLine, linePart)
				copy(fullLine[len(linePart):], remainder)
				remainder = nil
			} else {
				fullLine = linePart
			}

			chunk = chunk[:idx]
			trimmed := bytes.TrimRight(fullLine, "\r\n")
			if len(trimmed) == 0 {
				continue
			}

			scannedLines++
			if scannedLines > maxScanLines {
				break
			}

			if matchLine(trimmed) {
				lines = append(lines, string(trimmed))
			}
		}
	}

	if len(remainder) > 0 && len(lines) < maxLines && scannedLines < maxScanLines && scannedBytes < maxScanBytes {
		trimmed := bytes.TrimRight(remainder, "\r\n")
		if len(trimmed) > 0 && matchLine(trimmed) {
			lines = append(lines, string(trimmed))
		}
	}

	if len(lines) > maxLines {
		lines = lines[:maxLines]
	}

	// 反转恢复为时间正序（从旧到新）
	for i, j := 0, len(lines)-1; i < j; i, j = i+1, j-1 {
		lines[i], lines[j] = lines[j], lines[i]
	}

	return lines, nil
}

func ParseAccessLogLine(line string) *AccessLogEntry {
	trimmed := strings.TrimSpace(line)
	if trimmed == "" {
		return nil
	}

	m := accessLogRe.FindStringSubmatch(trimmed)
	if len(m) >= 5 {
		fullTarget := m[3]
		proto := "TCP"
		targetHost := fullTarget
		if strings.HasPrefix(fullTarget, "tcp:") {
			proto = "TCP"
			targetHost = strings.TrimPrefix(fullTarget, "tcp:")
		} else if strings.HasPrefix(fullTarget, "udp:") {
			proto = "UDP"
			targetHost = strings.TrimPrefix(fullTarget, "udp:")
		}

		action := "accepted"
		if strings.Contains(trimmed, "rejected") {
			action = "rejected"
		}

		email := ""
		if len(m) >= 6 {
			email = m[5]
		}

		route := m[4]
		inboundTag := route
		outboundTag := ""
		if strings.Contains(route, "->") {
			parts := strings.SplitN(route, "->", 2)
			inboundTag = strings.TrimSpace(parts[0])
			outboundTag = strings.TrimSpace(parts[1])
		}

		return &AccessLogEntry{
			Time:        m[1],
			FromIP:      m[2],
			Protocol:    proto,
			Target:      targetHost,
			Route:       route,
			InboundTag:  inboundTag,
			OutboundTag: outboundTag,
			Email:       email,
			Action:      action,
			Raw:         trimmed,
		}
	}

	if strings.Contains(trimmed, "DOH") || strings.Contains(trimmed, "answer:") || strings.Contains(trimmed, "dns-out") {
		timeStr := ""
		if len(trimmed) >= 19 {
			timeStr = trimmed[:19]
		}
		target := trimmed
		if idx := strings.Index(trimmed, "answer:"); idx != -1 {
			target = trimmed[idx:]
		}
		return &AccessLogEntry{
			Time:     timeStr,
			FromIP:   "127.0.0.1 (DNS)",
			Protocol: "DNS",
			Target:   target,
			Route:    "dns-resolver",
			Email:    "",
			Action:   "query",
			Raw:      trimmed,
		}
	}

	return nil
}

func ParseErrorLogLine(line string) *ErrorLogEntry {
	trimmed := strings.TrimSpace(line)
	if trimmed == "" {
		return nil
	}

	m := errorLogRe.FindStringSubmatch(trimmed)
	if len(m) >= 5 {
		lvl := strings.ToUpper(m[2])
		if lvl == "WARNING" {
			lvl = "WARN"
		}
		msg := m[4]

		smartTip := ""
		msgLower := strings.ToLower(msg)
		if strings.Contains(msgLower, "client flow is empty") {
			smartTip = "客户端未配置 Vision 流控，或当前入站协议不为 TCP"
		} else if strings.Contains(msgLower, "dns-query") && (strings.Contains(msgLower, "context canceled") || strings.Contains(msgLower, "timeout")) {
			smartTip = "DoH 远端解析握手超时，建议优先使用 8.8.8.8 UDP DNS"
		} else if strings.Contains(msgLower, "nokerneltun") {
			smartTip = "WireGuard WARP 采用用户态 gVisor TUN 启动成功"
		} else if strings.Contains(msgLower, "started") {
			smartTip = "Xray 核心服务已成功初始化并加载配置"
		}

		return &ErrorLogEntry{
			Time:     m[1],
			Level:    lvl,
			Module:   m[3],
			Message:  msg,
			SmartTip: smartTip,
			Raw:      trimmed,
		}
	}

	return nil
}
