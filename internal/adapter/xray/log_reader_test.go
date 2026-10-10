package xray

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

func TestReadLastLinesFiltered_Basic(t *testing.T) {
	tmpDir := t.TempDir()

	// 1. 测试常规文件及末尾截取
	logFile := filepath.Join(tmpDir, "test.log")
	f, err := os.Create(logFile)
	if err != nil {
		t.Fatalf("create temp log file failed: %v", err)
	}
	for i := 1; i <= 200; i++ {
		_, _ = f.WriteString(fmt.Sprintf("log line %03d\n", i))
	}
	f.Close()

	lines, err := ReadLastLinesFiltered(logFile, 5, LogFilter{})
	if err != nil {
		t.Fatalf("ReadLastLinesFiltered failed: %v", err)
	}

	if len(lines) != 5 {
		t.Fatalf("expected 5 lines, got %d", len(lines))
	}

	if lines[0] != "log line 196" || lines[4] != "log line 200" {
		t.Errorf("unexpected lines content: %v", lines)
	}

	// 2. 测试请求行数大于总行数
	allLines, err := ReadLastLinesFiltered(logFile, 500, LogFilter{})
	if err != nil {
		t.Fatalf("ReadLastLinesFiltered with large maxLines failed: %v", err)
	}
	if len(allLines) != 200 {
		t.Errorf("expected 200 lines, got %d", len(allLines))
	}
	if allLines[0] != "log line 001" || allLines[199] != "log line 200" {
		t.Errorf("unexpected allLines content: %v ... %v", allLines[0], allLines[len(allLines)-1])
	}

	// 3. 测试空文件
	emptyFile := filepath.Join(tmpDir, "empty.log")
	fEmpty, err := os.Create(emptyFile)
	if err != nil {
		t.Fatalf("create empty file failed: %v", err)
	}
	fEmpty.Close()

	emptyLines, err := ReadLastLinesFiltered(emptyFile, 10, LogFilter{})
	if err != nil {
		t.Fatalf("ReadLastLinesFiltered on empty file failed: %v", err)
	}
	if len(emptyLines) != 0 {
		t.Errorf("expected 0 lines from empty file, got %d", len(emptyLines))
	}

	// 4. 测试不存在的文件
	nonExistent := filepath.Join(tmpDir, "non_existent.log")
	_, err = ReadLastLinesFiltered(nonExistent, 10, LogFilter{})
	if err == nil {
		t.Errorf("expected error for non-existent file, got nil")
	}

	// 5. 测试过滤读取与标签提取
	mixedFile := filepath.Join(tmpDir, "mixed.log")
	fMixed, _ := os.Create(mixedFile)
	fMixed.WriteString("2026/09/12 10:00:01 127.0.0.1:1000 accepted tcp:example.com:443 [vless-in -> direct] email: alice@test.com\n")
	fMixed.WriteString("2026/09/12 10:00:02 127.0.0.1:1001 accepted tcp:google.com:443 [vmess-in -> warp-out] email: bob@test.com\n")
	fMixed.WriteString("2026/09/12 10:00:03 127.0.0.1:1002 accepted tcp:youtube.com:443 [vless-in -> direct] email: carol@test.com\n")
	fMixed.Close()

	filtered, err := ReadLastLinesFiltered(mixedFile, 10, LogFilter{InboundTag: "vless-in"})
	if err != nil {
		t.Fatalf("ReadLastLinesFiltered failed: %v", err)
	}
	if len(filtered) != 2 {
		t.Fatalf("expected 2 filtered lines, got %d", len(filtered))
	}

	entry := ParseAccessLogLine(filtered[0])
	if entry == nil || entry.InboundTag != "vless-in" || entry.OutboundTag != "direct" {
		t.Fatalf("unexpected parsed entry: %+v", entry)
	}
}

func TestReadLastLinesFiltered_BufferPoolConcurrent(t *testing.T) {
	tmpDir := t.TempDir()
	logFile := filepath.Join(tmpDir, "concurrent_test.log")
	f, err := os.Create(logFile)
	if err != nil {
		t.Fatalf("create temp log file failed: %v", err)
	}
	for i := 1; i <= 1000; i++ {
		_, _ = f.WriteString(fmt.Sprintf("2026/09/12 10:00:%02d 127.0.0.1:%04d accepted tcp:target%d.com:443 [vless-in -> direct] email: user%d@test.com\n", i%60, 1000+i, i, i))
	}
	f.Close()

	var wg sync.WaitGroup
	const goroutines = 30
	wg.Add(goroutines)

	for i := 0; i < goroutines; i++ {
		go func(gid int) {
			defer wg.Done()
			lines, err := ReadLastLinesFiltered(logFile, 20, LogFilter{InboundTag: "vless-in"})
			if err != nil {
				t.Errorf("concurrent ReadLastLinesFiltered failed: %v", err)
				return
			}
			if len(lines) != 20 {
				t.Errorf("expected 20 lines, got %d", len(lines))
				return
			}
			if !strings.Contains(lines[len(lines)-1], "user1000@test.com") {
				t.Errorf("unexpected last line: %s", lines[len(lines)-1])
			}
		}(i)
	}
	wg.Wait()
}

func TestParseLogLineFast(t *testing.T) {
	line := []byte("2026/09/12 10:00:01 127.0.0.1:1000 accepted tcp:example.com:443 [vless-in -> direct] email: alice@test.com\n")

	matched, timeStr, fromIP, target := ParseLogLineFast(line, LogFilter{InboundTag: "vless-in"})
	if !matched {
		t.Fatalf("expected line to match filter")
	}
	if timeStr != "2026/09/12 10:00:01" || fromIP != "127.0.0.1:1000" || target != "tcp:example.com:443" {
		t.Fatalf("unexpected parsed result: time=%s, ip=%s, target=%s", timeStr, fromIP, target)
	}

	// 不匹配标签测试
	matched, _, _, _ = ParseLogLineFast(line, LogFilter{InboundTag: "vmess-in"})
	if matched {
		t.Fatalf("expected line to NOT match different inbound tag")
	}

	// 关键字过滤测试
	matched, _, _, _ = ParseLogLineFast(line, LogFilter{Keyword: "ALICE"})
	if !matched {
		t.Fatalf("expected line to match case-insensitive keyword")
	}
}

func TestReadLastLinesFiltered_AnonymousAndTagBoundary(t *testing.T) {
	tmpDir := t.TempDir()
	logFile := filepath.Join(tmpDir, "access_boundary.log")
	f, err := os.Create(logFile)
	if err != nil {
		t.Fatalf("create temp log file failed: %v", err)
	}

	// 写入合成日志：包含匿名 TCP accepted、匿名 UDP rejected、具有相似前缀的 local-other 入口
	f.WriteString("2026/10/10 12:00:01 127.0.0.1:51234 accepted tcp:1.1.1.1:53 [local -> direct]\n")
	f.WriteString("2026/10/10 12:00:02 127.0.0.1:51235 rejected udp:8.8.8.8:53 [local -> block]\n")
	f.WriteString("2026/10/10 12:00:03 127.0.0.1:51236 accepted tcp:example.com:443 [local-other -> direct]\n")
	f.WriteString("2026/10/10 12:00:04 127.0.0.1:51237 accepted tcp:api.internal:80 [local]\n")
	f.WriteString("2026/10/10 12:00:05 127.0.0.1:51238 accepted tcp:fallback.internal:80 [local >> direct]\n")
	f.Close()

	// 1. 过滤 local 标签：必须只返回 local 的 4 行，严禁混入 local-other
	lines, err := ReadLastLinesFiltered(logFile, 10, LogFilter{InboundTag: "local"})
	if err != nil {
		t.Fatalf("ReadLastLinesFiltered failed: %v", err)
	}
	if len(lines) != 4 {
		t.Fatalf("expected exactly 4 lines for 'local' tag, got %d: %v", len(lines), lines)
	}

	// 2. 验证解析匿名 TCP accepted 条目
	entry1 := ParseAccessLogLine(lines[0])
	if entry1 == nil {
		t.Fatalf("failed to parse line 0: %s", lines[0])
	}
	if entry1.Email != "" {
		t.Errorf("expected empty email for anonymous entry, got %s", entry1.Email)
	}
	if entry1.Protocol != "TCP" || entry1.Target != "1.1.1.1:53" || entry1.InboundTag != "local" || entry1.Action != "accepted" {
		t.Errorf("unexpected parsed entry1: %+v", entry1)
	}

	// 3. 验证解析匿名 UDP rejected 条目
	entry2 := ParseAccessLogLine(lines[1])
	if entry2 == nil {
		t.Fatalf("failed to parse line 1: %s", lines[1])
	}
	if entry2.Email != "" {
		t.Errorf("expected empty email for anonymous entry, got %s", entry2.Email)
	}
	if entry2.Protocol != "UDP" || entry2.Target != "8.8.8.8:53" || entry2.InboundTag != "local" || entry2.Action != "rejected" {
		t.Errorf("unexpected parsed entry2: %+v", entry2)
	}

	// 4. 验证解析单 tag 路由 [local]
	entry3 := ParseAccessLogLine(lines[2])
	if entry3 == nil {
		t.Fatalf("failed to parse line 2: %s", lines[2])
	}
	if entry3.InboundTag != "local" {
		t.Errorf("expected InboundTag 'local', got %s", entry3.InboundTag)
	}

	// 5. 过滤 local-other 标签：必须只返回 1 行
	otherLines, err := ReadLastLinesFiltered(logFile, 10, LogFilter{InboundTag: "local-other"})
	if err != nil {
		t.Fatalf("ReadLastLinesFiltered failed: %v", err)
	}
	if len(otherLines) != 1 {
		t.Fatalf("expected 1 line for 'local-other', got %d: %v", len(otherLines), otherLines)
	}
	entryOther := ParseAccessLogLine(otherLines[0])
	if entryOther == nil || entryOther.InboundTag != "local-other" {
		t.Errorf("unexpected parsed entryOther: %+v", entryOther)
	}
}

func TestParseAccessLogLine_Comprehensive(t *testing.T) {
	tests := []struct {
		name       string
		line       string
		wantNil    bool
		wantTime   string
		wantFromIP string
		wantProto  string
		wantTarget string
		wantRoute  string
		wantInb    string
		wantOutb   string
		wantEmail  string
		wantAction string
	}{
		{
			name:       "standard access log with email",
			line:       "2026/09/12 10:00:01 127.0.0.1:1000 accepted tcp:example.com:443 [vless-in -> direct] email: alice@test.com",
			wantTime:   "2026/09/12 10:00:01",
			wantFromIP: "127.0.0.1:1000",
			wantProto:  "TCP",
			wantTarget: "example.com:443",
			wantRoute:  "vless-in -> direct",
			wantInb:    "vless-in",
			wantOutb:   "direct",
			wantEmail:  "alice@test.com",
			wantAction: "accepted",
		},
		{
			name:       "anonymous access log (no email)",
			line:       "2026/09/12 10:00:01 127.0.0.1:1000 accepted tcp:example.com:443 [vless-in -> direct]",
			wantTime:   "2026/09/12 10:00:01",
			wantFromIP: "127.0.0.1:1000",
			wantProto:  "TCP",
			wantTarget: "example.com:443",
			wantRoute:  "vless-in -> direct",
			wantInb:    "vless-in",
			wantOutb:   "direct",
			wantEmail:  "",
			wantAction: "accepted",
		},
		{
			name:       "access log with milliseconds timestamp",
			line:       "2026/09/12 10:00:01.789 127.0.0.1:1000 accepted tcp:example.com:443 [vless-in -> direct] email: alice@test.com",
			wantTime:   "2026/09/12 10:00:01",
			wantFromIP: "127.0.0.1:1000",
			wantProto:  "TCP",
			wantTarget: "example.com:443",
			wantRoute:  "vless-in -> direct",
			wantInb:    "vless-in",
			wantOutb:   "direct",
			wantEmail:  "alice@test.com",
			wantAction: "accepted",
		},
		{
			name:       "access log with IPv6 address",
			line:       "2026/09/12 10:00:01 [::1]:51234 accepted tcp:example.com:443 [vless-in -> direct]",
			wantTime:   "2026/09/12 10:00:01",
			wantFromIP: "[::1]:51234",
			wantProto:  "TCP",
			wantTarget: "example.com:443",
			wantRoute:  "vless-in -> direct",
			wantInb:    "vless-in",
			wantOutb:   "direct",
			wantEmail:  "",
			wantAction: "accepted",
		},
		{
			name:       "access log with IPv6 target",
			line:       "2026/09/12 10:00:01 127.0.0.1:1000 accepted tcp:[2001:db8::1]:443 [vless-in -> direct]",
			wantTime:   "2026/09/12 10:00:01",
			wantFromIP: "127.0.0.1:1000",
			wantProto:  "TCP",
			wantTarget: "[2001:db8::1]:443",
			wantRoute:  "vless-in -> direct",
			wantInb:    "vless-in",
			wantOutb:   "direct",
			wantEmail:  "",
			wantAction: "accepted",
		},
		{
			name:       "access log with from prefix and udp prefix",
			line:       "2026/09/12 10:00:01 from udp:127.0.0.1:1000 accepted tcp:example.com:443 [vless-in -> direct]",
			wantTime:   "2026/09/12 10:00:01",
			wantFromIP: "127.0.0.1:1000",
			wantProto:  "TCP",
			wantTarget: "example.com:443",
			wantRoute:  "vless-in -> direct",
			wantInb:    "vless-in",
			wantOutb:   "direct",
			wantEmail:  "",
			wantAction: "accepted",
		},
		{
			name:       "access log with from prefix and IPv6",
			line:       "2026/09/12 10:00:01 from tcp:[2001:db8::1]:51234 accepted tcp:example.com:443 [vless-in -> direct]",
			wantTime:   "2026/09/12 10:00:01",
			wantFromIP: "[2001:db8::1]:51234",
			wantProto:  "TCP",
			wantTarget: "example.com:443",
			wantRoute:  "vless-in -> direct",
			wantInb:    "vless-in",
			wantOutb:   "direct",
			wantEmail:  "",
			wantAction: "accepted",
		},
		{
			name:       "access log with >> route and rejected action",
			line:       "2026/09/12 10:00:01 127.0.0.1:1000 rejected udp:8.8.8.8:53 [vless-in >> block]",
			wantTime:   "2026/09/12 10:00:01",
			wantFromIP: "127.0.0.1:1000",
			wantProto:  "UDP",
			wantTarget: "8.8.8.8:53",
			wantRoute:  "vless-in >> block",
			wantInb:    "vless-in",
			wantOutb:   "block",
			wantEmail:  "",
			wantAction: "rejected",
		},
		{
			name:       "access log with single tag route",
			line:       "2026/09/12 10:00:01 127.0.0.1:1000 accepted tcp:api.internal:80 [local]",
			wantTime:   "2026/09/12 10:00:01",
			wantFromIP: "127.0.0.1:1000",
			wantProto:  "TCP",
			wantTarget: "api.internal:80",
			wantRoute:  "local",
			wantInb:    "local",
			wantOutb:   "",
			wantEmail:  "",
			wantAction: "accepted",
		},
		{
			name:       "special DNS log line with answer:",
			line:       "2026/09/12 10:00:01 [dns-out] 127.0.0.1:53 DOH query answer: google.com",
			wantTime:   "2026/09/12 10:00:01",
			wantFromIP: "127.0.0.1 (DNS)",
			wantProto:  "DNS",
			wantTarget: "answer: google.com",
			wantRoute:  "dns-resolver",
			wantInb:    "",
			wantOutb:   "",
			wantEmail:  "",
			wantAction: "query",
		},
		{
			name:       "special DNS log line with dns-out",
			line:       "2026/09/12 10:00:01 dns-out to 8.8.8.8",
			wantTime:   "2026/09/12 10:00:01",
			wantFromIP: "127.0.0.1 (DNS)",
			wantProto:  "DNS",
			wantTarget: "2026/09/12 10:00:01 dns-out to 8.8.8.8",
			wantRoute:  "dns-resolver",
			wantInb:    "",
			wantOutb:   "",
			wantEmail:  "",
			wantAction: "query",
		},
		{
			name:    "invalid empty line",
			line:    "   ",
			wantNil: true,
		},
		{
			name:    "invalid random line",
			line:    "this is not a valid xray log line",
			wantNil: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			entry := ParseAccessLogLine(tt.line)
			if tt.wantNil {
				if entry != nil {
					t.Fatalf("expected nil entry, got %+v", entry)
				}
				return
			}
			if entry == nil {
				t.Fatalf("expected non-nil entry, got nil")
			}
			if entry.Time != tt.wantTime {
				t.Errorf("Time = %q, want %q", entry.Time, tt.wantTime)
			}
			if entry.FromIP != tt.wantFromIP {
				t.Errorf("FromIP = %q, want %q", entry.FromIP, tt.wantFromIP)
			}
			if entry.Protocol != tt.wantProto {
				t.Errorf("Protocol = %q, want %q", entry.Protocol, tt.wantProto)
			}
			if entry.Target != tt.wantTarget {
				t.Errorf("Target = %q, want %q", entry.Target, tt.wantTarget)
			}
			if entry.Route != tt.wantRoute {
				t.Errorf("Route = %q, want %q", entry.Route, tt.wantRoute)
			}
			if entry.InboundTag != tt.wantInb {
				t.Errorf("InboundTag = %q, want %q", entry.InboundTag, tt.wantInb)
			}
			if entry.OutboundTag != tt.wantOutb {
				t.Errorf("OutboundTag = %q, want %q", entry.OutboundTag, tt.wantOutb)
			}
			if entry.Email != tt.wantEmail {
				t.Errorf("Email = %q, want %q", entry.Email, tt.wantEmail)
			}
			if entry.Action != tt.wantAction {
				t.Errorf("Action = %q, want %q", entry.Action, tt.wantAction)
			}
		})
	}
}

func TestParseErrorLogLine_Comprehensive(t *testing.T) {
	tests := []struct {
		name         string
		line         string
		wantNil      bool
		wantTime     string
		wantLevel    string
		wantModule   string
		wantMessage  string
		wantSmartTip string
	}{
		{
			name:        "warning level normalized to WARN",
			line:        "2026/09/12 10:00:01 [Warning] Core: failed to process inbound connection",
			wantTime:    "2026/09/12 10:00:01",
			wantLevel:   "WARN",
			wantModule:  "Core",
			wantMessage: "failed to process inbound connection",
		},
		{
			name:         "info level with milliseconds and PID",
			line:         "2026/09/12 10:00:01.123 [Info] [12345] App/Proxyman/Inbound: Xray core started",
			wantTime:     "2026/09/12 10:00:01",
			wantLevel:    "INFO",
			wantModule:   "App/Proxyman/Inbound",
			wantMessage:  "Xray core started",
			wantSmartTip: "Xray 核心服务已成功初始化并加载配置",
		},
		{
			name:         "error with vision smart tip",
			line:         "2026/09/12 10:00:01 [Error] Proxy/VLESS: client flow is empty",
			wantTime:     "2026/09/12 10:00:01",
			wantLevel:    "ERROR",
			wantModule:   "Proxy/VLESS",
			wantMessage:  "client flow is empty",
			wantSmartTip: "客户端未配置 Vision 流控，或当前入站协议不为 TCP",
		},
		{
			name:         "error with dns timeout smart tip",
			line:         "2026/09/12 10:00:01 [Warning] DNS: dns-query request timeout",
			wantTime:     "2026/09/12 10:00:01",
			wantLevel:    "WARN",
			wantModule:   "DNS",
			wantMessage:  "dns-query request timeout",
			wantSmartTip: "DoH 远端解析握手超时，建议优先使用 8.8.8.8 UDP DNS",
		},
		{
			name:         "info with nokerneltun smart tip",
			line:         "2026/09/12 10:00:01 [Info] WireGuard: nokerneltun enabled",
			wantTime:     "2026/09/12 10:00:01",
			wantLevel:    "INFO",
			wantModule:   "WireGuard",
			wantMessage:  "nokerneltun enabled",
			wantSmartTip: "WireGuard WARP 采用用户态 gVisor TUN 启动成功",
		},
		{
			name:    "empty line returns nil",
			line:    "   ",
			wantNil: true,
		},
		{
			name:    "unformatted line returns nil",
			line:    "2026/09/12 10:00:01 malformed error log line",
			wantNil: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			entry := ParseErrorLogLine(tt.line)
			if tt.wantNil {
				if entry != nil {
					t.Fatalf("expected nil entry, got %+v", entry)
				}
				return
			}
			if entry == nil {
				t.Fatalf("expected non-nil entry, got nil")
			}
			if entry.Time != tt.wantTime {
				t.Errorf("Time = %q, want %q", entry.Time, tt.wantTime)
			}
			if entry.Level != tt.wantLevel {
				t.Errorf("Level = %q, want %q", entry.Level, tt.wantLevel)
			}
			if entry.Module != tt.wantModule {
				t.Errorf("Module = %q, want %q", entry.Module, tt.wantModule)
			}
			if entry.Message != tt.wantMessage {
				t.Errorf("Message = %q, want %q", entry.Message, tt.wantMessage)
			}
			if tt.wantSmartTip != "" && entry.SmartTip != tt.wantSmartTip {
				t.Errorf("SmartTip = %q, want %q", entry.SmartTip, tt.wantSmartTip)
			}
		})
	}
}

func TestParseLogLineFast_Comprehensive(t *testing.T) {
	line := []byte("2026/09/12 10:00:01.456 [::1]:51234 accepted tcp:example.com:443 [vless-in -> direct] email: alice@test.com\n")

	// 1. Tag match
	matched, timeStr, fromIP, target := ParseLogLineFast(line, LogFilter{InboundTag: "vless-in"})
	if !matched {
		t.Fatalf("expected matched")
	}
	if timeStr != "2026/09/12 10:00:01" || fromIP != "[::1]:51234" || target != "tcp:example.com:443" {
		t.Fatalf("unexpected extracted fields: time=%s, ip=%s, target=%s", timeStr, fromIP, target)
	}

	// 2. Tag mismatch
	matched, _, _, _ = ParseLogLineFast(line, LogFilter{InboundTag: "other-in"})
	if matched {
		t.Fatalf("expected not matched")
	}

	// 3. Keyword match (case insensitive)
	matched, _, _, _ = ParseLogLineFast(line, LogFilter{Keyword: "ALICE@TEST.COM"})
	if !matched {
		t.Fatalf("expected keyword matched")
	}

	// 4. Keyword mismatch
	matched, _, _, _ = ParseLogLineFast(line, LogFilter{Keyword: "non-existent"})
	if matched {
		t.Fatalf("expected keyword not matched")
	}

	// 5. Empty line
	matched, _, _, _ = ParseLogLineFast([]byte("   \n"), LogFilter{})
	if matched {
		t.Fatalf("expected empty line to not match")
	}
}

func BenchmarkParseAccessLogLine(b *testing.B) {
	line := "2026/09/12 10:00:01.123 127.0.0.1:1000 accepted tcp:example.com:443 [vless-in -> direct] email: alice@test.com"
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		entry := ParseAccessLogLine(line)
		if entry == nil {
			b.Fatal("nil entry")
		}
	}
}

func BenchmarkParseErrorLogLine(b *testing.B) {
	line := "2026/09/12 10:00:01.456 [Warning] [12345] Core: failed to process inbound connection"
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		entry := ParseErrorLogLine(line)
		if entry == nil {
			b.Fatal("nil entry")
		}
	}
}

func BenchmarkParseLogLineFast(b *testing.B) {
	line := []byte("2026/09/12 10:00:01.123 127.0.0.1:1000 accepted tcp:example.com:443 [vless-in -> direct] email: alice@test.com")
	filter := LogFilter{InboundTag: "vless-in"}
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		matched, _, _, _ := ParseLogLineFast(line, filter)
		if !matched {
			b.Fatal("not matched")
		}
	}
}
