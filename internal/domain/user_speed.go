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
		// 3分钟内有流量活跃即判定为在线连接
		online := (now-r.LastActive < 180000) && r.LastActive > 0
		curUp := r.UpSpeed
		curDown := r.DownSpeed
		// 若超过 30 秒无新增数据，实时瞬时速率归零
		if now-r.LastActive > 30000 {
			curUp = 0
			curDown = 0
		}
		return curUp, curDown, r.LastActive, online
	}
	return 0, 0, 0, false
}
