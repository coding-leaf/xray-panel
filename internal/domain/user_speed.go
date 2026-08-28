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
	rec.LastActive = lastActive
}

func GetUserRuntimeSpeed(email string) (upSpeed, downSpeed, lastActive int64, isOnline bool) {
	speedTrackerMu.RLock()
	defer speedTrackerMu.RUnlock()
	if r, ok := speedTracker[email]; ok {
		now := time.Now().UnixMilli()
		online := (now-r.LastActive < 60000) && (r.UpSpeed > 0 || r.DownSpeed > 0)
		return r.UpSpeed, r.DownSpeed, r.LastActive, online
	}
	return 0, 0, 0, false
}
