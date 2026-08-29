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

var (
	speedTrackerMu sync.RWMutex
	speedTracker   = make(map[string]*UserSpeedRecord)
)

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
