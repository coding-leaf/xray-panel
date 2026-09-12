package xray

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

func TestReadLastLines(t *testing.T) {
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

	lines, err := ReadLastLines(logFile, 5)
	if err != nil {
		t.Fatalf("ReadLastLines failed: %v", err)
	}

	if len(lines) != 5 {
		t.Fatalf("expected 5 lines, got %d", len(lines))
	}

	if lines[0] != "log line 196" || lines[4] != "log line 200" {
		t.Errorf("unexpected lines content: %v", lines)
	}

	// 2. 测试请求行数大于总行数
	allLines, err := ReadLastLines(logFile, 500)
	if err != nil {
		t.Fatalf("ReadLastLines with large maxLines failed: %v", err)
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

	emptyLines, err := ReadLastLines(emptyFile, 10)
	if err != nil {
		t.Fatalf("ReadLastLines on empty file failed: %v", err)
	}
	if len(emptyLines) != 0 {
		t.Errorf("expected 0 lines from empty file, got %d", len(emptyLines))
	}

	// 4. 测试不存在的文件
	nonExistent := filepath.Join(tmpDir, "non_existent.log")
	_, err = ReadLastLines(nonExistent, 10)
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
