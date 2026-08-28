package xray

import (
	"bufio"
	"fmt"
	"os"
)

// ReadLastLines 从文件末尾读取最后的 N 行日志
func ReadLastLines(filePath string, maxLines int) ([]string, error) {
	if maxLines <= 0 {
		maxLines = 100
	}
	if maxLines > 1000 {
		maxLines = 1000
	}

	file, err := os.Open(filePath)
	if err != nil {
		return nil, fmt.Errorf("open log file failed: %w", err)
	}
	defer file.Close()

	var lines []string
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		lines = append(lines, scanner.Text())
		if len(lines) > maxLines {
			lines = lines[1:]
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("scan log lines failed: %w", err)
	}

	return lines, nil
}
