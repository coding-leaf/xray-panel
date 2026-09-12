package monitor

import (
	"context"
	"testing"
	"time"
)

func TestGopsutilMonitor_BasicAndConcurrency(t *testing.T) {
	m := NewGopsutilMonitor()

	ctx := context.Background()
	metrics, err := m.GetSystemMetrics(ctx)
	if err != nil {
		t.Fatalf("GetSystemMetrics failed: %v", err)
	}
	if metrics == nil {
		t.Fatal("expected non-nil metrics on initialization")
	}

	// Verify Start and graceful stop with context cancellation
	startCtx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	errCh := make(chan error, 1)
	go func() {
		errCh <- m.Start(startCtx)
	}()

	select {
	case err := <-errCh:
		if err != nil {
			t.Fatalf("Start returned unexpected error: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Start did not terminate within timeout")
	}
}
