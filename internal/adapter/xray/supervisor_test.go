package xray

import (
	"context"
	"sync"
	"testing"
	"time"
)

func TestSystemdSupervisor_ConcurrentAccess(t *testing.T) {
	sup := NewSystemdSupervisor("xray", "/nonexistent/xray-bin")

	const workers = 8
	const iters = 30
	var wg sync.WaitGroup

	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for i := 0; i < iters; i++ {
				ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
				switch (id + i) % 4 {
				case 0:
					sup.UpdateConfig("xray-svc", "/nonexistent/xray-bin-alt")
				case 1:
					_, _ = sup.GetStatus(ctx)
				case 2:
					_ = sup.Restart(ctx)
				case 3:
					_, _ = sup.GetVersion(ctx)
				}
				cancel()
			}
		}(w)
	}

	wg.Wait()
}
