package domain_test

import (
	"fmt"
	"sync"
	"testing"
	"time"

	"panel/internal/domain"
)

func TestUserTrafficReset_TimeWindowFilter(t *testing.T) {
	email := "reset_test@example.com"

	// Initially not recently reset
	if domain.IsUserRecentlyReset(email, 6000) {
		t.Fatalf("expected user to not be recently reset initially")
	}

	// Set speed and record reset
	domain.SetUserRuntimeSpeed(email, 500, 1000, time.Now().UnixMilli())
	domain.RecordUserTrafficReset(email)

	// Immediately after reset, IsUserRecentlyReset must be true
	if !domain.IsUserRecentlyReset(email, 6000) {
		t.Fatalf("expected user to be marked as recently reset")
	}

	// Runtime speed should be reset to 0
	up, down, _, _ := domain.GetUserRuntimeSpeed(email)
	if up != 0 || down != 0 {
		t.Errorf("expected runtime speed to be 0 after reset, got up=%d, down=%d", up, down)
	}

	// After removing user, reset record should be cleaned up
	domain.RemoveUserRuntimeSpeed(email)
	if domain.IsUserRecentlyReset(email, 6000) {
		t.Fatalf("expected user reset record to be cleaned up on remove")
	}
}

func TestBatchUpdateUserRuntimeSpeeds_Concurrency(t *testing.T) {
	const goroutines = 20
	const iterations = 50

	var wg sync.WaitGroup
	wg.Add(goroutines * 2)

	for i := 0; i < goroutines; i++ {
		go func(gid int) {
			defer wg.Done()
			for it := 0; it < iterations; it++ {
				email1 := fmt.Sprintf("u%d_1@test.com", gid)
				email2 := fmt.Sprintf("u%d_2@test.com", gid)
				deltas := []domain.UserTrafficDeltaUpdate{
					{Email: email1, Up: int64(100 * (it + 1)), Down: int64(200 * (it + 1))},
					{Email: email2, Up: int64(300 * (it + 1)), Down: int64(400 * (it + 1))},
				}
				domain.BatchUpdateUserRuntimeSpeeds(deltas, 2, time.Now().UnixMilli())
			}
		}(i)

		go func() {
			defer wg.Done()
			for it := 0; it < iterations; it++ {
				_ = domain.GetAllUserRuntimeSpeeds()
			}
		}()
	}

	wg.Wait()
}

func TestBatchUpdateUserRuntimeSpeeds_CleanExpired(t *testing.T) {
	now := time.Now().UnixMilli()

	// 1. 设置用户并记录 reset 时间（模拟 20 秒前 reset）
	domain.RecordUserTrafficReset("old_reset@test.com")
	// 覆盖其 reset 时间为 20 秒前
	// 通过 BatchUpdateUserRuntimeSpeeds 触发清理
	deltas := []domain.UserTrafficDeltaUpdate{
		{Email: "active@test.com", Up: 1000, Down: 2000},
	}
	// 传 now + 20000ms 模拟经过了 20 秒
	domain.BatchUpdateUserRuntimeSpeeds(deltas, 1, now+20000)

	// old_reset 应该被淘汰出 userResetTimes
	if domain.IsUserRecentlyReset("old_reset@test.com", 60000) {
		t.Fatalf("expected old_reset to be cleaned up after batch update")
	}

	// 2. 检查长期无流量用户淘汰 (超过 10 分钟无活跃)
	domain.SetUserRuntimeSpeed("inactive@test.com", 0, 0, now)
	// 模拟当前时间已过 15 分钟 (15 * 60 * 1000 ms)
	domain.BatchUpdateUserRuntimeSpeeds(deltas, 1, now+15*60*1000)

	speeds := domain.GetAllUserRuntimeSpeeds()
	if _, ok := speeds["inactive@test.com"]; ok {
		t.Fatalf("expected inactive user to be evicted from speedTracker")
	}
}

func TestCalculateSpeedDelta(t *testing.T) {
	up, down := domain.CalculateSpeedDelta(1000, 2000, 2)
	if up != 500 || down != 1000 {
		t.Fatalf("expected up=500, down=1000, got up=%d, down=%d", up, down)
	}

	// 除以 0 或负数测试
	up, down = domain.CalculateSpeedDelta(1000, 2000, 0)
	if up != 1000 || down != 2000 {
		t.Fatalf("expected up=1000, down=2000 for interval=0, got up=%d, down=%d", up, down)
	}
}


