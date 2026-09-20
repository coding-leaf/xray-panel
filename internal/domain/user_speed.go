package domain

import (
	"sync"
	"time"
)

type UserSpeedRecord struct {
	UpSpeed    int64
	DownSpeed  int64
	LastActive int64
}

type UserSpeedStatus struct {
	Email      string `json:"email"`
	UpSpeed    int64  `json:"upSpeed"`
	DownSpeed  int64  `json:"downSpeed"`
	LastActive int64  `json:"lastActive"`
	IsOnline   bool   `json:"isOnline"`
}

type UserTrafficDeltaUpdate struct {
	Email string
	Up    int64
	Down  int64
}

var (
	speedTrackerMu sync.RWMutex
	speedTracker   = make(map[string]*UserSpeedRecord)
	userResetTimes = make(map[string]int64)
)

// CalculateSpeedDelta 速率计算纯函数，含除以零与异常保护
func CalculateSpeedDelta(upBytes, downBytes, intervalSec int64) (upSpeed, downSpeed int64) {
	if intervalSec <= 0 {
		intervalSec = 1
	}
	if upBytes < 0 {
		upBytes = 0
	}
	if downBytes < 0 {
		downBytes = 0
	}
	return upBytes / intervalSec, downBytes / intervalSec
}

// BatchUpdateUserRuntimeSpeeds 单次获取写锁，原子完成全量用户速率计算、无流量清零及过期缓存淘汰
func BatchUpdateUserRuntimeSpeeds(deltas []UserTrafficDeltaUpdate, intervalSec int64, nowMs int64) {
	if nowMs <= 0 {
		nowMs = time.Now().UnixMilli()
	}
	if intervalSec <= 0 {
		intervalSec = 1
	}

	speedTrackerMu.Lock()
	defer speedTrackerMu.Unlock()

	activeEmails := make(map[string]struct{}, len(deltas))
	for _, d := range deltas {
		activeEmails[d.Email] = struct{}{}
		rec, ok := speedTracker[d.Email]
		if !ok {
			rec = &UserSpeedRecord{}
			speedTracker[d.Email] = rec
		}
		upSpeed, downSpeed := CalculateSpeedDelta(d.Up, d.Down, intervalSec)
		if d.Up > 0 || d.Down > 0 {
			rec.UpSpeed = upSpeed
			rec.DownSpeed = downSpeed
			rec.LastActive = nowMs
		} else {
			rec.UpSpeed = 0
			rec.DownSpeed = 0
		}
	}

	// 对本轮未提供增量的用户，瞬时速率置零；若超过 10 分钟无活跃流量，则淘汰回收内存
	const inactiveEvictMs = 10 * 60 * 1000 // 10 分钟
	for email, rec := range speedTracker {
		if _, ok := activeEmails[email]; !ok {
			rec.UpSpeed = 0
			rec.DownSpeed = 0
			if rec.LastActive > 0 && (nowMs-rec.LastActive > inactiveEvictMs) {
				delete(speedTracker, email)
			}
		}
	}

	// 顺带清理 userResetTimes 中超过 15 秒的历史记录，防止 map 内存泄漏
	const resetExpireMs = 15 * 1000 // 15 秒
	for email, resetTime := range userResetTimes {
		if nowMs-resetTime > resetExpireMs {
			delete(userResetTimes, email)
		}
	}
}


func SetUserRuntimeSpeed(email string, upSpeed, downSpeed, lastActive int64) {
	speedTrackerMu.Lock()
	defer speedTrackerMu.Unlock()
	rec := speedTracker[email]
	if rec == nil {
		rec = &UserSpeedRecord{}
		speedTracker[email] = rec
	}
	rec.UpSpeed = upSpeed
	rec.DownSpeed = downSpeed
	if lastActive > 0 {
		rec.LastActive = lastActive
	}
}

// RecordUserTrafficReset 记录用户流量重置时刻（Unix 毫秒），并清空当前瞬时速率
func RecordUserTrafficReset(email string) {
	speedTrackerMu.Lock()
	defer speedTrackerMu.Unlock()
	userResetTimes[email] = time.Now().UnixMilli()
	if rec := speedTracker[email]; rec != nil {
		rec.UpSpeed = 0
		rec.DownSpeed = 0
	}
}

// IsUserRecentlyReset 检查用户是否在最近 windowMs 毫秒内刚被重置（在窗口期内过滤在途残留增量）
func IsUserRecentlyReset(email string, windowMs int64) bool {
	speedTrackerMu.RLock()
	defer speedTrackerMu.RUnlock()
	resetTime, ok := userResetTimes[email]
	if !ok {
		return false
	}
	return (time.Now().UnixMilli() - resetTime) < windowMs
}

// RemoveUserRuntimeSpeed 从内存中安全清理已删除用户的速率追踪状态，防止幽灵对象常驻内存泄漏
func RemoveUserRuntimeSpeed(email string) {
	speedTrackerMu.Lock()
	defer speedTrackerMu.Unlock()
	delete(speedTracker, email)
	delete(userResetTimes, email)
}

func GetUserRuntimeSpeed(email string) (upSpeed, downSpeed, lastActive int64, isOnline bool) {
	speedTrackerMu.RLock()
	defer speedTrackerMu.RUnlock()
	if r, ok := speedTracker[email]; ok {
		now := time.Now().UnixMilli()
		// 20秒内有流量活跃即判定为在线连接
		online := (now-r.LastActive < 20000) && r.LastActive > 0
		curUp := r.UpSpeed
		curDown := r.DownSpeed
		// 若超过 6 秒无新增数据，实时瞬时速率立即归零
		if now-r.LastActive > 6000 {
			curUp = 0
			curDown = 0
		}
		return curUp, curDown, r.LastActive, online
	}
	return 0, 0, 0, false
}

// GetAllUserRuntimeSpeeds 从内存中毫秒级快照所有用户的当前速率与在线状态（0 数据库 I/O）
func GetAllUserRuntimeSpeeds() map[string]UserSpeedStatus {
	speedTrackerMu.RLock()
	defer speedTrackerMu.RUnlock()

	now := time.Now().UnixMilli()
	result := make(map[string]UserSpeedStatus, len(speedTracker))

	for email, r := range speedTracker {
		online := (now-r.LastActive < 20000) && r.LastActive > 0
		curUp := r.UpSpeed
		curDown := r.DownSpeed
		if now-r.LastActive > 6000 {
			curUp = 0
			curDown = 0
		}
		result[email] = UserSpeedStatus{
			Email:      email,
			UpSpeed:    curUp,
			DownSpeed:  curDown,
			LastActive: r.LastActive,
			IsOnline:   online,
		}
	}
	return result
}
