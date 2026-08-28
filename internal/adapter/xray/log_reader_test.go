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
}
