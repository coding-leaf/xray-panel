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
