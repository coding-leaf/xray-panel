package cron_test

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	deliveryCron "panel/internal/delivery/cron"
	"panel/internal/domain"
)

type mockRealityInspector struct {
	checkCalls int64
	mu         sync.Mutex
}

func (m *mockRealityInspector) CheckAll(ctx context.Context) (domain.RealitySummaryStatus, error) {
	atomic.AddInt64(&m.checkCalls, 1)
	return domain.RealitySummaryStatus{
		TotalChecked: 1,
		OkCount:      1,
	}, nil
}

func TestRealitySyncJob_Lifecycle(t *testing.T) {
	inspector := &mockRealityInspector{}
	interval := 30 * time.Millisecond
	job := deliveryCron.NewRealitySyncJob(inspector, interval)

	ctx, cancel := context.WithCancel(context.Background())
	errCh := make(chan error, 1)

	go func() {
		errCh <- job.Start(ctx)
	}()

	// 等待初始探测和至少一次定时周期触发
	time.Sleep(80 * time.Millisecond)

	// 取消 context 触发优雅退出
	cancel()

	select {
	case err := <-errCh:
		if err != nil {
			t.Fatalf("job.Start returned unexpected error: %v", err)
		}
	case <-time.After(1 * time.Second):
		t.Fatal("job.Start failed to stop within 1s")
	}

	calls := atomic.LoadInt64(&inspector.checkCalls)
	if calls < 1 {
		t.Fatalf("expected at least 1 inspection call, got %d", calls)
	}

	// 验证 Stop 方法
	if err := job.Stop(context.Background()); err != nil {
		t.Fatalf("job.Stop returned error: %v", err)
	}
}
