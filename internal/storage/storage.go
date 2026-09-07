package storage

import (
	"errors"
	"fmt"

	"panel/internal/domain"
)

// Bucket names defined for bbolt database
const (
	BucketUsersTraffic   = "users_traffic"
	BucketTrafficHistory = "traffic_history"
)

// Sentinel errors
var (
	ErrUserNotFound     = fmt.Errorf("user traffic not found: %w", domain.ErrNotFound)
	ErrInvalidTimeRange = fmt.Errorf("invalid time range: %w", domain.ErrInvalidInput)
	ErrEmptyEmail       = fmt.Errorf("empty email: %w", domain.ErrInvalidInput)
	ErrDatabaseClosed   = errors.New("storage: database is closed")
)

// TrafficDelta represents incremental traffic for a user during a sync period.
type TrafficDelta struct {
	Uplink    int64 `json:"uplink"`
	Downlink  int64 `json:"downlink"`
	Timestamp int64 `json:"timestamp,omitempty"` // Unix timestamp in seconds; 0 defaults to current time
}

// UserTraffic stores cumulative traffic statistics for a user.
type UserTraffic struct {
	Email    string `json:"email"`
	Uplink   int64  `json:"uplink"`
	Downlink int64  `json:"downlink"`
	LastSeen int64  `json:"lastSeen"` // Unix timestamp in seconds
}

// TrafficSnapshot represents a point-in-time or aggregated traffic sample.
type TrafficSnapshot struct {
	Email     string `json:"email,omitempty"`
	Timestamp int64  `json:"timestamp"` // Unix timestamp in seconds
	Uplink    int64  `json:"uplink"`
	Downlink  int64  `json:"downlink"`
}

// Storage defines the interface for traffic storage operations.
type Storage interface {
	// SaveTrafficBatch updates multiple users' cumulative traffic and appends/aggregates time series snapshots in a single transaction.
	SaveTrafficBatch(records map[string]TrafficDelta) error

	// GetUserTraffic retrieves cumulative traffic statistics for a specific user.
	GetUserTraffic(email string) (*UserTraffic, error)

	// GetAllUsersTraffic retrieves cumulative traffic statistics for all users.
	GetAllUsersTraffic() ([]*UserTraffic, error)

	// GetTrafficHistory retrieves time series sampling points for a user within [startTime, endTime] (inclusive).
	GetTrafficHistory(email string, startTime, endTime int64) ([]TrafficSnapshot, error)

	// ResetUserTraffic resets cumulative traffic (Uplink and Downlink) for a user to zero.
	ResetUserTraffic(email string) error

	// DeleteUserTraffic removes a user's cumulative traffic and all historical records.
	DeleteUserTraffic(email string) error

	// Close safely closes the storage engine.
	Close() error
}
