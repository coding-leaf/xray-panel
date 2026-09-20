package service

import (
	"context"
	"fmt"
	"os"

	"panel/internal/domain"
)

type LogReader interface {
	GetLogPaths() (accessLog, errorLog string)
	ReadLastLinesFiltered(filePath string, maxLines int, filter domain.LogFilter) ([]string, error)
	ParseAccessLog(line string) *domain.AccessLogEntry
	ParseErrorLog(line string) *domain.ErrorLogEntry
}

type LogService struct {
	reader LogReader
}

func NewLogService(reader LogReader) *LogService {
	return &LogService{reader: reader}
}

type LogResponse struct {
	Type     string                  `json:"type"`
	FilePath string                  `json:"filePath"`
	Lines    []string                `json:"lines"`
	Access   []domain.AccessLogEntry `json:"access,omitempty"`
	Errors   []domain.ErrorLogEntry  `json:"errors,omitempty"`
}

func (s *LogService) GetRecentLogs(ctx context.Context, logType string, maxLines int, filters ...domain.LogFilter) (*LogResponse, error) {
	accessPath, errorPath := s.reader.GetLogPaths()

	var targetPath string
	if logType == "error" {
		targetPath = errorPath
	} else {
		logType = "access"
		targetPath = accessPath
	}

	if targetPath == "" {
		return &LogResponse{
			Type:     logType,
			FilePath: "未在 config.json 的 log 字段中配置",
			Lines:    []string{"提示: 当前配置文件中未指定 log.access 或 log.error 路径"},
		}, nil
	}

	filter := domain.LogFilter{}
	if len(filters) > 0 {
		filter = filters[0]
	}

	lines, err := s.reader.ReadLastLinesFiltered(targetPath, maxLines, filter)
	if err != nil {
		return &LogResponse{
			Type:     logType,
			FilePath: targetPath,
			Lines:    []string{fmt.Sprintf("读取日志文件失败: %v", err)},
		}, nil
	}

	resp := &LogResponse{
		Type:     logType,
		FilePath: targetPath,
		Lines:    lines,
	}

	if logType == "access" {
		entries := make([]domain.AccessLogEntry, 0, len(lines))
		for _, l := range lines {
			if e := s.reader.ParseAccessLog(l); e != nil {
				entries = append(entries, *e)
			}
		}
		resp.Access = entries
	} else {
		entries := make([]domain.ErrorLogEntry, 0, len(lines))
		for _, l := range lines {
			if e := s.reader.ParseErrorLog(l); e != nil {
				entries = append(entries, *e)
			}
		}
		resp.Errors = entries
	}

	return resp, nil
}

// ClearLogs 安全截断日志文件（大小置为 0）
func (s *LogService) ClearLogs(ctx context.Context, logType string) error {
	accessPath, errorPath := s.reader.GetLogPaths()

	var targetPath string
	if logType == "error" {
		targetPath = errorPath
	} else {
		targetPath = accessPath
	}

	if targetPath == "" {
		return fmt.Errorf("当前未配置 %s 日志路径", logType)
	}

	if err := os.Truncate(targetPath, 0); err != nil {
		return fmt.Errorf("清空日志文件失败: %w", err)
	}

	return nil
}
