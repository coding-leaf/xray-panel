package service

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"panel/internal/adapter/reality"
	"panel/internal/domain"
	"panel/internal/pkg/logger"
)

// RealityMonitorService handles periodic and on-demand health inspections for Reality disguise domains.
type RealityMonitorService struct {
	inboundRepo  domain.InboundRepository
	prober       reality.RealityProber
	alertService *AlertService
	mu           sync.RWMutex
	cachedStatus domain.RealitySummaryStatus
}

// NewRealityMonitorService creates a new RealityMonitorService.
func NewRealityMonitorService(
	inboundRepo domain.InboundRepository,
	prober reality.RealityProber,
	alertService *AlertService,
) *RealityMonitorService {
	return &RealityMonitorService{
		inboundRepo:  inboundRepo,
		prober:       prober,
		alertService: alertService,
		cachedStatus: domain.RealitySummaryStatus{
			Items: []domain.RealityCheckItem{},
		},
	}
}

// GetStatus returns the cached Reality check summary in sub-millisecond read lock time.
func (s *RealityMonitorService) GetStatus() domain.RealitySummaryStatus {
	s.mu.RLock()
	defer s.mu.RUnlock()

	res := s.cachedStatus
	if res.Items != nil {
		items := make([]domain.RealityCheckItem, len(res.Items))
		copy(items, res.Items)
		res.Items = items
	} else {
		res.Items = []domain.RealityCheckItem{}
	}
	return res
}

type realityTask struct {
	inboundID  uint
	inboundTag string
	dest       string
	port       int
	serverName string
}

// CheckAll inspects all Reality inbound configurations, probes destinations with limited concurrency,
// updates the in-memory cache, and dispatches alerts for abnormal items.
func (s *RealityMonitorService) CheckAll(ctx context.Context) (domain.RealitySummaryStatus, error) {
	if s.inboundRepo == nil {
		return s.GetStatus(), nil
	}

	inbounds, err := s.inboundRepo.ListAll(ctx)
	if err != nil {
		return s.GetStatus(), fmt.Errorf("list all inbounds failed: %w", err)
	}

	// 1. 纯解析提取所有待探测的 Reality 任务
	var tasks []realityTask
	for _, in := range inbounds {
		dest, port, serverNames, isReality := domain.ExtractRealityTargets(&in)
		if !isReality {
			continue
		}
		if len(serverNames) == 0 && dest != "" {
			serverNames = []string{dest}
		}
		for _, sn := range serverNames {
			tasks = append(tasks, realityTask{
				inboundID:  in.ID,
				inboundTag: in.Tag,
				dest:       dest,
				port:       port,
				serverName: sn,
			})
		}
	}

	now := time.Now()
	if len(tasks) == 0 {
		emptySummary := domain.RealitySummaryStatus{
			TotalChecked: 0,
			TotalCount:   0,
			OkCount:      0,
			WarningCount: 0,
			ErrorCount:   0,
			Items:        []domain.RealityCheckItem{},
			LastCheckAt:  now,
			CheckedAt:    now,
		}
		s.mu.Lock()
		s.cachedStatus = emptySummary
		s.mu.Unlock()
		return emptySummary, nil
	}

	// 2. 受限并发探测 (最大 5 并发)
	const maxConcurrency = 5
	sem := make(chan struct{}, maxConcurrency)
	results := make([]domain.RealityCheckItem, len(tasks))

	var wg sync.WaitGroup
	for i, t := range tasks {
		wg.Add(1)
		go func(idx int, task realityTask) {
			defer wg.Done()

			select {
			case sem <- struct{}{}:
				defer func() { <-sem }()
			case <-ctx.Done():
				results[idx] = domain.BuildRealityCheckItem(
					task.inboundID,
					task.inboundTag,
					task.dest,
					task.port,
					task.serverName,
					domain.RealityStatusError,
					domain.ErrTypeTCPUnreachable,
					fmt.Sprintf("探测因上下文取消终止: %v", ctx.Err()),
					0,
					"",
					nil,
					false,
					0,
					now,
				)
				return
			}

			var probeRes *reality.ProbeResult
			var probeErr error
			if s.prober != nil {
				probeRes, probeErr = s.prober.Probe(ctx, task.dest, task.port, task.serverName)
			}
			if probeRes == nil {
				probeRes = &reality.ProbeResult{
					TCPError: probeErr,
					Headers:  make(http.Header),
				}
			} else if probeRes.TCPError == nil && probeErr != nil {
				probeRes.TCPError = probeErr
			}

			status, errType, details := domain.EvaluateRealityProbe(
				probeRes.TCPError,
				probeRes.TLSVersion,
				probeRes.ALPN,
				probeRes.Cert,
				probeRes.Headers,
				now,
				task.serverName,
			)

			isCDN := errType == domain.ErrTypeCDNDetected
			item := domain.BuildRealityCheckItem(
				task.inboundID,
				task.inboundTag,
				task.dest,
				task.port,
				task.serverName,
				status,
				errType,
				details,
				probeRes.TLSVersion,
				probeRes.ALPN,
				probeRes.Cert,
				isCDN,
				probeRes.LatencyMs,
				now,
			)

			if (item.Status == domain.RealityStatusWarning || item.Status == domain.RealityStatusError) && s.alertService != nil {
				if err := s.alertService.NotifyRealityAbnormal(ctx, item); err != nil {
					logger.FromContext(ctx).Warn("Notify reality abnormal failed",
						slog.String("inbound", item.InboundTag),
						slog.String("server_name", item.ServerName),
						slog.String("error", err.Error()),
					)
				}
			}

			results[idx] = item
		}(i, t)
	}

	wg.Wait()

	// 3. 统计结果聚合
	var okCount, warningCount, errorCount int
	for _, it := range results {
		switch it.Status {
		case domain.RealityStatusOk:
			okCount++
		case domain.RealityStatusWarning:
			warningCount++
		case domain.RealityStatusError:
			errorCount++
		}
	}

	summary := domain.RealitySummaryStatus{
		TotalChecked: len(results),
		TotalCount:   len(results),
		OkCount:      okCount,
		WarningCount: warningCount,
		ErrorCount:   errorCount,
		Items:        results,
		LastCheckAt:  now,
		CheckedAt:    now,
	}

	s.mu.Lock()
	s.cachedStatus = summary
	s.mu.Unlock()

	return summary, nil
}
