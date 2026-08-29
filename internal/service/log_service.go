package service

import (
	"context"
	"fmt"

	"panel/internal/adapter/xray"
)

type LogService struct {
	configMgr *xray.ConfigManager
}

func NewLogService(configMgr *xray.ConfigManager) *LogService {
	return &LogService{configMgr: configMgr}
}

type LogResponse struct {
	Type     string   `json:"type"`
	FilePath string   `json:"filePath"`
	Lines    []string `json:"lines"`
}

func (s *LogService) GetRecentLogs(ctx context.Context, logType string, maxLines int) (*LogResponse, error) {
	accessPath, errorPath := s.configMgr.GetLogPaths()

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

	lines, err := xray.ReadLastLines(targetPath, maxLines)
	if err != nil {
		return &LogResponse{
			Type:     logType,
			FilePath: targetPath,
			Lines:    []string{fmt.Sprintf("读取日志文件失败: %v", err)},
		}, nil
	}

	return &LogResponse{
		Type:     logType,
		FilePath: targetPath,
		Lines:    lines,
	}, nil
}
