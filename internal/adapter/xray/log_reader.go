package xray

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"strings"
	"sync"

	"panel/internal/domain"
)

var (
	logBufferPool = sync.Pool{
		New: func() any {
			b := make([]byte, 64*1024)
			return &b
		},
	}
)

type AccessLogEntry = domain.AccessLogEntry
type ErrorLogEntry = domain.ErrorLogEntry
type LogFilter = domain.LogFilter

func isDigit(b byte) bool {
	return b >= '0' && b <= '9'
}

func isTimestamp(s string) bool {
	if len(s) < 19 {
		return false
	}
	return isDigit(s[0]) && isDigit(s[1]) && isDigit(s[2]) && isDigit(s[3]) &&
		s[4] == '/' &&
		isDigit(s[5]) && isDigit(s[6]) &&
		s[7] == '/' &&
		isDigit(s[8]) && isDigit(s[9]) &&
		s[10] == ' ' &&
		isDigit(s[11]) && isDigit(s[12]) &&
		s[13] == ':' &&
		isDigit(s[14]) && isDigit(s[15]) &&
		s[16] == ':' &&
		isDigit(s[17]) && isDigit(s[18])
}

func isTimestampBytes(s []byte) bool {
	if len(s) < 19 {
		return false
	}
	return isDigit(s[0]) && isDigit(s[1]) && isDigit(s[2]) && isDigit(s[3]) &&
		s[4] == '/' &&
		isDigit(s[5]) && isDigit(s[6]) &&
		s[7] == '/' &&
		isDigit(s[8]) && isDigit(s[9]) &&
		s[10] == ' ' &&
		isDigit(s[11]) && isDigit(s[12]) &&
		s[13] == ':' &&
		isDigit(s[14]) && isDigit(s[15]) &&
		s[16] == ':' &&
		isDigit(s[17]) && isDigit(s[18])
}

// containsFoldASCII 高性能零分配 ASCII 大小写无关子串匹配
func containsFoldASCII(s []byte, sub string) bool {
	if len(sub) == 0 {
		return true
	}
	if len(s) < len(sub) {
		return false
	}

	for i := 0; i < len(sub); i++ {
		if sub[i] >= 128 {
			return bytes.Contains(bytes.ToLower(s), []byte(strings.ToLower(sub)))
		}
	}

	sub0Lower := sub[0]
	if sub0Lower >= 'A' && sub0Lower <= 'Z' {
		sub0Lower += 'a' - 'A'
	}
	sub0Upper := sub[0]
	if sub0Upper >= 'a' && sub0Upper <= 'z' {
		sub0Upper -= 'a' - 'A'
	}

	max := len(s) - len(sub)
	for i := 0; i <= max; i++ {
		c := s[i]
		if c != sub0Lower && c != sub0Upper {
			continue
		}
		matched := true
		for j := 1; j < len(sub); j++ {
			sc := s[i+j]
			tc := sub[j]
			if sc >= 'A' && sc <= 'Z' {
				sc += 'a' - 'A'
			}
			if tc >= 'A' && tc <= 'Z' {
				tc += 'a' - 'A'
			}
			if sc != tc {
				matched = false
				break
			}
		}
		if matched {
			return true
		}
	}
	return false
}

var (
	arrowBytes         = []byte("->")
	doubleGreaterBytes = []byte(">>")
)

// equalBytesString 高性能零分配字节切片与字符串比较
func equalBytesString(b []byte, s string) bool {
	if len(b) != len(s) {
		return false
	}
	for i := 0; i < len(b); i++ {
		if b[i] != s[i] {
			return false
		}
	}
	return true
}

// equalFoldBytesString 高性能零分配字节切片与字符串大小写无关比较
func equalFoldBytesString(b []byte, s string) bool {
	if len(b) != len(s) {
		return false
	}
	for i := 0; i < len(b); i++ {
		c1 := b[i]
		c2 := s[i]
		if c1 >= 'A' && c1 <= 'Z' {
			c1 += 'a' - 'A'
		}
		if c2 >= 'A' && c2 <= 'Z' {
			c2 += 'a' - 'A'
		}
		if c1 != c2 {
			return false
		}
	}
	return true
}

// matchInboundTagBytes 高性能零分配入站标签匹配，支持 [tag]、[tag -> ...] 与 [tag >> ...]
func matchInboundTagBytes(line []byte, tag string) bool {
	if tag == "" {
		return true
	}
	offset := 0
	for {
		idx := bytes.IndexByte(line[offset:], '[')
		if idx == -1 {
			return false
		}
		start := offset + idx + 1
		rem := line[start:]
		if len(rem) >= len(tag) && equalBytesString(rem[:len(tag)], tag) {
			after := rem[len(tag):]
			if len(after) > 0 && after[0] == ']' {
				if len(after) == 1 || after[1] != ':' {
					return true
				}
			}
			if len(after) >= 3 && after[0] == ' ' && after[1] == '-' && after[2] == '>' {
				return true
			}
			if len(after) >= 3 && after[0] == ' ' && after[1] == '>' && after[2] == '>' {
				return true
			}
		}
		offset = start
	}
}

// matchOutboundTagBytes 高性能零分配出站标签匹配，支持 [in -> out]、[in >> out] 与独立 [out]
func matchOutboundTagBytes(line []byte, tag string) bool {
	if tag == "" {
		return true
	}
	offset := 0
	for {
		idx := bytes.IndexByte(line[offset:], '[')
		if idx == -1 {
			return false
		}
		start := offset + idx + 1
		closeIdx := bytes.IndexByte(line[start:], ']')
		if closeIdx == -1 {
			return false
		}
		// 跳过 IPv6 host:port 形式的括号（如 [::1]:51234），防止误识别为路由标签
		if start+closeIdx+1 < len(line) && line[start+closeIdx+1] == ':' {
			offset = start + closeIdx + 1
			continue
		}
		bracketContent := line[start : start+closeIdx]

		if arrowIdx := bytes.Index(bracketContent, arrowBytes); arrowIdx != -1 {
			outboundPart := bytes.TrimSpace(bracketContent[arrowIdx+2:])
			if equalBytesString(outboundPart, tag) {
				return true
			}
		} else if arrowIdx := bytes.Index(bracketContent, doubleGreaterBytes); arrowIdx != -1 {
			outboundPart := bytes.TrimSpace(bracketContent[arrowIdx+2:])
			if equalBytesString(outboundPart, tag) {
				return true
			}
		} else {
			outboundPart := bytes.TrimSpace(bracketContent)
			if equalBytesString(outboundPart, tag) {
				return true
			}
		}

		offset = start + closeIdx + 1
	}
}

// matchLevelBytes 高性能零分配错误日志级别匹配，支持 WARN/WARNING, INFO, ERROR, DEBUG 等大小写无关比对
func matchLevelBytes(line []byte, level string) bool {
	level = strings.TrimSpace(level)
	if level == "" {
		return true
	}
	isWarn := strings.EqualFold(level, "WARN") || strings.EqualFold(level, "WARNING")
	var target string
	if isWarn {
		target = "WARN"
	} else if strings.EqualFold(level, "INFO") {
		target = "INFO"
	} else if strings.EqualFold(level, "ERROR") {
		target = "ERROR"
	} else if strings.EqualFold(level, "DEBUG") {
		target = "DEBUG"
	} else if strings.EqualFold(level, "NONE") {
		target = "NONE"
	} else {
		return false
	}

	offset := 0
	for {
		idx := bytes.IndexByte(line[offset:], '[')
		if idx == -1 {
			return false
		}
		start := offset + idx + 1
		closeIdx := bytes.IndexByte(line[start:], ']')
		if closeIdx == -1 {
			return false
		}
		candidate := bytes.TrimSpace(line[start : start+closeIdx])

		if isWarn {
			if equalFoldBytesString(candidate, "WARN") || equalFoldBytesString(candidate, "WARNING") {
				return true
			}
		} else if equalFoldBytesString(candidate, target) {
			return true
		}

		offset = start + closeIdx + 1
	}
}

// scanAccessLogMetaBytes 基于字节切片的高速元信息提取，零堆分配
func scanAccessLogMetaBytes(b []byte) (timeStr, fromIP, target string) {
	if !isTimestampBytes(b) {
		return "", "", ""
	}
	timeBytes := b[:19]
	pos := 19
	if pos < len(b) && b[pos] == '.' {
		pos++
		for pos < len(b) && isDigit(b[pos]) {
			pos++
		}
	}
	if pos >= len(b) || b[pos] != ' ' {
		return "", "", ""
	}
	for pos < len(b) && b[pos] == ' ' {
		pos++
	}

	// [from ][tcp:|udp:]
	if bytes.HasPrefix(b[pos:], []byte("from ")) {
		pos += 5
		for pos < len(b) && b[pos] == ' ' {
			pos++
		}
	}
	if bytes.HasPrefix(b[pos:], []byte("tcp:")) || bytes.HasPrefix(b[pos:], []byte("udp:")) {
		pos += 4
	}

	// FromIP
	spaceIdx := bytes.IndexByte(b[pos:], ' ')
	if spaceIdx == -1 {
		return "", "", ""
	}
	ipBytes := b[pos : pos+spaceIdx]
	pos += spaceIdx + 1
	for pos < len(b) && b[pos] == ' ' {
		pos++
	}

	// Action: accepted / rejected
	if bytes.HasPrefix(b[pos:], []byte("accepted")) {
		pos += 8
	} else if bytes.HasPrefix(b[pos:], []byte("rejected")) {
		pos += 8
	} else {
		return "", "", ""
	}
	if pos >= len(b) || b[pos] != ' ' {
		return "", "", ""
	}
	for pos < len(b) && b[pos] == ' ' {
		pos++
	}

	// Target: followed by " ["
	bracketIdx := bytes.Index(b[pos:], []byte(" ["))
	if bracketIdx == -1 {
		return "", "", ""
	}
	targetBytes := b[pos : pos+bracketIdx]

	closeBracket := bytes.IndexByte(b[pos+bracketIdx+2:], ']')
	if closeBracket == -1 {
		return "", "", ""
	}

	return string(timeBytes), string(ipBytes), string(targetBytes)
}

// ParseLogLineFast 基于字节数组的高速过滤与元信息提取，零堆分配
func ParseLogLineFast(raw []byte, filter LogFilter) (matched bool, timeStr, fromIP, target string) {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 {
		return false, "", "", ""
	}

	if filter.InboundTag != "" && !matchInboundTagBytes(trimmed, filter.InboundTag) {
		return false, "", "", ""
	}

	if filter.OutboundTag != "" && !matchOutboundTagBytes(trimmed, filter.OutboundTag) {
		return false, "", "", ""
	}

	if filter.Level != "" && !matchLevelBytes(trimmed, filter.Level) {
		return false, "", "", ""
	}

	if filter.Keyword != "" && !containsFoldASCII(trimmed, filter.Keyword) {
		return false, "", "", ""
	}

	timeStr, fromIP, target = scanAccessLogMetaBytes(trimmed)
	return true, timeStr, fromIP, target
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

	matchLine := func(line []byte) bool {
		if filter.InboundTag != "" && !matchInboundTagBytes(line, filter.InboundTag) {
			return false
		}
		if filter.OutboundTag != "" && !matchOutboundTagBytes(line, filter.OutboundTag) {
			return false
		}
		if filter.Level != "" && !matchLevelBytes(line, filter.Level) {
			return false
		}
		if filter.Keyword != "" && !containsFoldASCII(line, filter.Keyword) {
			return false
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

	if isTimestamp(trimmed) {
		timeStr := trimmed[:19]
		pos := 19
		if pos < len(trimmed) && trimmed[pos] == '.' {
			pos++
			for pos < len(trimmed) && isDigit(trimmed[pos]) {
				pos++
			}
		}
		if pos < len(trimmed) && trimmed[pos] == ' ' {
			for pos < len(trimmed) && trimmed[pos] == ' ' {
				pos++
			}

			// Check optional "from "
			if strings.HasPrefix(trimmed[pos:], "from ") {
				pos += 5
				for pos < len(trimmed) && trimmed[pos] == ' ' {
					pos++
				}
			}

			// Check optional "tcp:" or "udp:"
			if strings.HasPrefix(trimmed[pos:], "tcp:") || strings.HasPrefix(trimmed[pos:], "udp:") {
				pos += 4
			}

			// FromIP
			spaceIdx := strings.IndexByte(trimmed[pos:], ' ')
			if spaceIdx != -1 {
				fromIP := trimmed[pos : pos+spaceIdx]
				pos += spaceIdx + 1
				for pos < len(trimmed) && trimmed[pos] == ' ' {
					pos++
				}

				// Action
				action := ""
				if strings.HasPrefix(trimmed[pos:], "accepted") {
					action = "accepted"
					pos += 8
				} else if strings.HasPrefix(trimmed[pos:], "rejected") {
					action = "rejected"
					pos += 8
				}

				if action != "" && pos < len(trimmed) && trimmed[pos] == ' ' {
					for pos < len(trimmed) && trimmed[pos] == ' ' {
						pos++
					}

					bracketIdx := strings.Index(trimmed[pos:], " [")
					if bracketIdx != -1 {
						fullTarget := trimmed[pos : pos+bracketIdx]
						pos += bracketIdx + 2 // skip " ["

						closeBracket := strings.IndexByte(trimmed[pos:], ']')
						if closeBracket != -1 {
							route := trimmed[pos : pos+closeBracket]
							rem := trimmed[pos+closeBracket+1:]

							proto := "TCP"
							targetHost := fullTarget
							if strings.HasPrefix(fullTarget, "tcp:") {
								proto = "TCP"
								targetHost = fullTarget[4:]
							} else if strings.HasPrefix(fullTarget, "udp:") {
								proto = "UDP"
								targetHost = fullTarget[4:]
							}

							email := ""
							remTrimmed := strings.TrimSpace(rem)
							const emailPrefix = "email:"
							if strings.HasPrefix(remTrimmed, emailPrefix) {
								email = strings.TrimSpace(remTrimmed[len(emailPrefix):])
							}

							inboundTag := route
							outboundTag := ""
							if idx := strings.Index(route, "->"); idx != -1 {
								inboundTag = strings.TrimSpace(route[:idx])
								outboundTag = strings.TrimSpace(route[idx+2:])
							} else if idx := strings.Index(route, ">>"); idx != -1 {
								inboundTag = strings.TrimSpace(route[:idx])
								outboundTag = strings.TrimSpace(route[idx+2:])
							}

							return &AccessLogEntry{
								Time:        timeStr,
								FromIP:      fromIP,
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
					}
				}
			}
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
	if len(trimmed) < 19 || !isTimestamp(trimmed) {
		return nil
	}

	timeStr := trimmed[:19]
	pos := 19
	if pos < len(trimmed) && trimmed[pos] == '.' {
		pos++
		for pos < len(trimmed) && isDigit(trimmed[pos]) {
			pos++
		}
	}
	if pos >= len(trimmed) || trimmed[pos] != ' ' {
		return nil
	}
	for pos < len(trimmed) && trimmed[pos] == ' ' {
		pos++
	}

	// Expect level in brackets [Level]
	if pos >= len(trimmed) || trimmed[pos] != '[' {
		return nil
	}
	levelClose := strings.IndexByte(trimmed[pos+1:], ']')
	if levelClose == -1 {
		return nil
	}
	levelRaw := trimmed[pos+1 : pos+1+levelClose]
	pos = pos + 1 + levelClose + 1

	if pos >= len(trimmed) || trimmed[pos] != ' ' {
		return nil
	}
	for pos < len(trimmed) && trimmed[pos] == ' ' {
		pos++
	}

	// Optional PID: [12345]
	if pos < len(trimmed) && trimmed[pos] == '[' {
		pidClose := strings.IndexByte(trimmed[pos+1:], ']')
		if pidClose != -1 {
			pidCandidate := trimmed[pos+1 : pos+1+pidClose]
			isPID := len(pidCandidate) > 0
			for i := 0; i < len(pidCandidate); i++ {
				if !isDigit(pidCandidate[i]) {
					isPID = false
					break
				}
			}
			if isPID {
				pos = pos + 1 + pidClose + 1
				if pos >= len(trimmed) || trimmed[pos] != ' ' {
					return nil
				}
				for pos < len(trimmed) && trimmed[pos] == ' ' {
					pos++
				}
			}
		}
	}

	// Module: Message
	colonIdx := strings.IndexByte(trimmed[pos:], ':')
	if colonIdx == -1 {
		return nil
	}
	module := trimmed[pos : pos+colonIdx]
	pos += colonIdx + 1
	for pos < len(trimmed) && (trimmed[pos] == ' ' || trimmed[pos] == '\t') {
		pos++
	}
	message := trimmed[pos:]

	lvl := strings.ToUpper(levelRaw)
	if lvl == "WARNING" {
		lvl = "WARN"
	}

	smartTip := ""
	msgLower := strings.ToLower(message)
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
		Time:     timeStr,
		Level:    lvl,
		Module:   module,
		Message:  message,
		SmartTip: smartTip,
		Raw:      trimmed,
	}
}
