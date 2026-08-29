package xray

import (
	"bytes"
	"fmt"
	"io"
	"os"
)

// ReadLastLines 从文件末尾反向高效读取最后的 N 行日志（支持百兆/吉字节大文件秒级返回）
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

	stat, err := file.Stat()
	if err != nil {
		return nil, err
	}
	fileSize := stat.Size()
	if fileSize == 0 {
		return []string{}, nil
	}

	var (
		chunkSize int64 = 4096
		offset    int64 = fileSize
		buffer    []byte
	)

	for offset > 0 {
		readSize := chunkSize
		if offset < chunkSize {
			readSize = offset
		}
		offset -= readSize

		chunk := make([]byte, readSize)
		_, err := file.ReadAt(chunk, offset)
		if err != nil && err != io.EOF {
			return nil, err
		}

		buffer = append(chunk, buffer...)
		lineCount := bytes.Count(buffer, []byte("\n"))
		if lineCount > maxLines+1 {
			break
		}
	}

	rawLines := bytes.Split(buffer, []byte("\n"))
	var lines []string
	for _, l := range rawLines {
		trimmed := string(bytes.TrimRight(l, "\r\n"))
		if trimmed != "" {
			lines = append(lines, trimmed)
		}
	}

	if len(lines) > maxLines {
		lines = lines[len(lines)-maxLines:]
	}

	return lines, nil
}
