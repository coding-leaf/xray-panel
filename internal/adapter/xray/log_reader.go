package xray

import (
	"fmt"
	"io"
	"os"
	"regexp"
	"strings"
)

var (
	accessLogRe = regexp.MustCompile(`^(\d{4}/\d{2}/\d{2} \d{2}:\d{2}:\d{2})(?:\.\d+)? (?:from )?([\d\.:]+) (?:accepted|rejected) (.*?) \[(.*?)\](?: email: (.*?))?$`)
	errorLogRe  = regexp.MustCompile(`^(\d{4}/\d{2}/\d{2} \d{2}:\d{2}:\d{2})(?:\.\d+)? \[(.*?)\] (?:\[\d+\] )?(.*?):\s*(.*)$`)
)

type AccessLogEntry struct {
	Time     string `json:"time"`
	FromIP   string `json:"from_ip"`
	Protocol string `json:"protocol"`
	Target   string `json:"target"`
	Route    string `json:"route"`
	Email    string `json:"email"`
	Action   string `json:"action"`
	Raw      string `json:"raw"`
}

type ErrorLogEntry struct {
	Time     string `json:"time"`
	Level    string `json:"level"`
	Module   string `json:"module"`
	Message  string `json:"message"`
	SmartTip string `json:"smartTip"`
	Raw      string `json:"raw"`
}

// ReadLastLines 从文件末尾反向高效分块读取（64KB Chunk Buffer，百兆大文件毫秒级返回）
func ReadLastLines(filePath string, maxLines int) ([]string, error) {
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

	var lines []string
	bufSize := int64(64 * 1024)
	offset := fileSize
	remainder := ""

	for offset > 0 && len(lines) < maxLines+1 {
		readSize := bufSize
		if offset < readSize {
			readSize = offset
		}
		offset -= readSize
		buf := make([]byte, readSize)
		_, err := file.ReadAt(buf, offset)
		if err != nil && err != io.EOF {
			return nil, err
		}

		chunk := string(buf) + remainder
		parts := strings.Split(chunk, "\n")
		remainder = parts[0]
		for i := len(parts) - 1; i >= 1; i-- {
			line := strings.TrimRight(parts[i], "\r\n")
			if line != "" {
				lines = append(lines, line)
			}
		}
	}

	if remainder != "" && len(lines) < maxLines {
		trimmed := strings.TrimRight(remainder, "\r\n")
		if trimmed != "" {
			lines = append(lines, trimmed)
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

		return &AccessLogEntry{
			Time:     m[1],
			FromIP:   m[2],
			Protocol: proto,
			Target:   targetHost,
			Route:    m[4],
			Email:    email,
			Action:   action,
			Raw:      trimmed,
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
