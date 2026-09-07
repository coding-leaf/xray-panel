package storage

import (
	"bytes"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"panel/internal/domain"

	"go.etcd.io/bbolt"
)

// Ensure BoltStorage implements Storage interface.
var _ Storage = (*BoltStorage)(nil)

// Options holds configuration for BoltStorage.
type Options struct {
	BboltOptions   *bbolt.Options
	Serializer     SerializerType
	HourlyRollup   bool
	RollupInterval time.Duration
	FileMode       os.FileMode
}

// Option configures BoltStorage.
type Option func(*Options)

// WithBboltOptions sets custom bbolt.Options.
func WithBboltOptions(opts *bbolt.Options) Option {
	return func(o *Options) {
		o.BboltOptions = opts
	}
}

// WithSerializer sets the serializer type (binary or compact JSON).
func WithSerializer(st SerializerType) Option {
	return func(o *Options) {
		o.Serializer = st
	}
}

// WithHourlyRollup enables or disables aggregating snapshots into hourly buckets (1 hour rollup).
func WithHourlyRollup(enabled bool) Option {
	return func(o *Options) {
		o.HourlyRollup = enabled
		if enabled {
			o.RollupInterval = time.Hour
		} else {
			o.RollupInterval = 0
		}
	}
}

// WithRollupInterval sets a custom time-series aggregation bucket duration.
func WithRollupInterval(interval time.Duration) Option {
	return func(o *Options) {
		o.RollupInterval = interval
		o.HourlyRollup = interval > 0
	}
}

// WithFileMode sets the file permissions for the bolt database file.
func WithFileMode(mode os.FileMode) Option {
	return func(o *Options) {
		o.FileMode = mode
	}
}

// safeAddInt64 performs saturating addition to prevent integer overflow.
func safeAddInt64(a, b int64) int64 {
	if b > 0 && a > math.MaxInt64-b {
		return math.MaxInt64
	}
	return a + b
}

// BoltStorage implements the Storage interface backed by pure-Go bbolt.
type BoltStorage struct {
	db        *bbolt.DB
	path      string
	opts      Options
	closeOnce sync.Once
	closeErr  error
	closed    atomic.Bool
}

// Open opens or creates a bbolt database at the specified path and initializes the required buckets.
func Open(path string, opts ...Option) (*BoltStorage, error) {
	if strings.TrimSpace(path) == "" {
		return nil, fmt.Errorf("%w: path cannot be empty", domain.ErrInvalidInput)
	}

	options := Options{
		BboltOptions: &bbolt.Options{
			Timeout: 2 * time.Second,
		},
		Serializer:   SerializerBinary,
		HourlyRollup: false,
		FileMode:     0600,
	}
	for _, opt := range opts {
		opt(&options)
	}

	dir := filepath.Dir(path)
	if dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return nil, fmt.Errorf("failed to create directory %s: %w", dir, err)
		}
	}

	db, err := bbolt.Open(path, options.FileMode, options.BboltOptions)
	if err != nil {
		return nil, fmt.Errorf("failed to open bbolt db at %s: %w", path, err)
	}

	// Initialize buckets
	err = db.Update(func(tx *bbolt.Tx) error {
		if _, err := tx.CreateBucketIfNotExists([]byte(BucketUsersTraffic)); err != nil {
			return fmt.Errorf("failed to create bucket %s: %w", BucketUsersTraffic, err)
		}
		if _, err := tx.CreateBucketIfNotExists([]byte(BucketTrafficHistory)); err != nil {
			return fmt.Errorf("failed to create bucket %s: %w", BucketTrafficHistory, err)
		}
		return nil
	})
	if err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("failed to initialize buckets: %w", err)
	}

	return &BoltStorage{
		db:   db,
		path: path,
		opts: options,
	}, nil
}

// Close cleanly shuts down the bbolt database. It is safe for concurrent and repeated calls.
func (s *BoltStorage) Close() error {
	if s == nil {
		return nil
	}
	s.closeOnce.Do(func() {
		s.closed.Store(true)
		if s.db != nil {
			s.closeErr = s.db.Close()
		}
	})
	return s.closeErr
}

func (s *BoltStorage) isClosed() bool {
	return s == nil || s.closed.Load()
}

// DB returns the underlying *bbolt.DB instance.
func (s *BoltStorage) DB() *bbolt.DB {
	if s == nil {
		return nil
	}
	return s.db
}

// Path returns the path of the database file.
func (s *BoltStorage) Path() string {
	if s == nil {
		return ""
	}
	return s.path
}

// SaveTrafficBatch atomically updates multiple users' traffic and appends/aggregates history snapshots.
func (s *BoltStorage) SaveTrafficBatch(records map[string]TrafficDelta) error {
	if s == nil || s.isClosed() {
		return ErrDatabaseClosed
	}
	if len(records) == 0 {
		return nil
	}

	// Validate inputs before beginning transaction
	for email, delta := range records {
		if strings.TrimSpace(email) == "" {
			return ErrEmptyEmail
		}
		if strings.Contains(email, string(keyDelimiter)) {
			return fmt.Errorf("%w: email contains invalid null byte", domain.ErrInvalidInput)
		}
		if delta.Uplink < 0 || delta.Downlink < 0 {
			return fmt.Errorf("%w: traffic delta values must be non-negative", domain.ErrInvalidInput)
		}
		if delta.Timestamp < 0 {
			return fmt.Errorf("%w: timestamp cannot be negative", domain.ErrInvalidInput)
		}
	}

	now := time.Now().Unix()

	return s.db.Update(func(tx *bbolt.Tx) error {
		usersBucket := tx.Bucket([]byte(BucketUsersTraffic))
		if usersBucket == nil {
			return fmt.Errorf("bucket %s not found", BucketUsersTraffic)
		}
		historyBucket := tx.Bucket([]byte(BucketTrafficHistory))
		if historyBucket == nil {
			return fmt.Errorf("bucket %s not found", BucketTrafficHistory)
		}

		for email, delta := range records {
			sampleTime := delta.Timestamp
			if sampleTime == 0 {
				sampleTime = now
			}

			// 1. Update users_traffic
			emailKey := []byte(email)
			existingUserBytes := usersBucket.Get(emailKey)

			var userTraffic *UserTraffic
			if existingUserBytes != nil {
				var err error
				userTraffic, err = decodeUserTraffic(existingUserBytes)
				if err != nil {
					return fmt.Errorf("failed to decode user traffic for %s: %w", email, err)
				}
			} else {
				userTraffic = &UserTraffic{
					Email: email,
				}
			}

			hasTraffic := delta.Uplink > 0 || delta.Downlink > 0
			userTraffic.Email = email
			userTraffic.Uplink = safeAddInt64(userTraffic.Uplink, delta.Uplink)
			userTraffic.Downlink = safeAddInt64(userTraffic.Downlink, delta.Downlink)
			if hasTraffic && sampleTime > userTraffic.LastSeen {
				userTraffic.LastSeen = sampleTime
			}

			userBytes, err := encodeUserTraffic(userTraffic, s.opts.Serializer)
			if err != nil {
				return fmt.Errorf("failed to encode user traffic for %s: %w", email, err)
			}
			if err := usersBucket.Put(emailKey, userBytes); err != nil {
				return fmt.Errorf("failed to put user traffic for %s: %w", email, err)
			}

			// 2. Update traffic_history
			historyTime := sampleTime
			if s.opts.RollupInterval > 0 {
				intervalSec := int64(s.opts.RollupInterval.Seconds())
				if intervalSec > 0 {
					historyTime = (sampleTime / intervalSec) * intervalSec
				}
			} else if s.opts.HourlyRollup {
				historyTime = (sampleTime / 3600) * 3600
			}

			histKey := makeHistoryKey(email, historyTime)
			existingHistBytes := historyBucket.Get(histKey)

			var snapshot *TrafficSnapshot
			if existingHistBytes != nil {
				var err error
				snapshot, err = decodeTrafficSnapshot(existingHistBytes)
				if err != nil {
					return fmt.Errorf("failed to decode traffic snapshot for %s at %d: %w", email, historyTime, err)
				}
				snapshot.Uplink = safeAddInt64(snapshot.Uplink, delta.Uplink)
				snapshot.Downlink = safeAddInt64(snapshot.Downlink, delta.Downlink)
			} else {
				snapshot = &TrafficSnapshot{
					Email:     email,
					Timestamp: historyTime,
					Uplink:    delta.Uplink,
					Downlink:  delta.Downlink,
				}
			}
			snapshot.Email = email
			snapshot.Timestamp = historyTime

			histBytes, err := encodeTrafficSnapshot(snapshot, s.opts.Serializer)
			if err != nil {
				return fmt.Errorf("failed to encode traffic snapshot for %s: %w", email, err)
			}
			if err := historyBucket.Put(histKey, histBytes); err != nil {
				return fmt.Errorf("failed to put traffic snapshot for %s: %w", email, err)
			}
		}

		return nil
	})
}

// GetUserTraffic retrieves cumulative traffic stats for a single user.
func (s *BoltStorage) GetUserTraffic(email string) (*UserTraffic, error) {
	if s == nil || s.isClosed() {
		return nil, ErrDatabaseClosed
	}
	if strings.TrimSpace(email) == "" {
		return nil, ErrEmptyEmail
	}
	if strings.Contains(email, string(keyDelimiter)) {
		return nil, fmt.Errorf("%w: email contains invalid null byte", domain.ErrInvalidInput)
	}

	var userTraffic *UserTraffic
	err := s.db.View(func(tx *bbolt.Tx) error {
		usersBucket := tx.Bucket([]byte(BucketUsersTraffic))
		if usersBucket == nil {
			return fmt.Errorf("bucket %s not found", BucketUsersTraffic)
		}

		data := usersBucket.Get([]byte(email))
		if data == nil {
			return ErrUserNotFound
		}

		var err error
		userTraffic, err = decodeUserTraffic(data)
		if err != nil {
			return fmt.Errorf("failed to decode user traffic for %s: %w", email, err)
		}
		userTraffic.Email = email
		return nil
	})
	if err != nil {
		return nil, err
	}
	return userTraffic, nil
}

// GetAllUsersTraffic returns cumulative traffic statistics for all users.
func (s *BoltStorage) GetAllUsersTraffic() ([]*UserTraffic, error) {
	if s == nil || s.isClosed() {
		return nil, ErrDatabaseClosed
	}

	results := make([]*UserTraffic, 0)
	err := s.db.View(func(tx *bbolt.Tx) error {
		usersBucket := tx.Bucket([]byte(BucketUsersTraffic))
		if usersBucket == nil {
			return fmt.Errorf("bucket %s not found", BucketUsersTraffic)
		}

		keyCount := usersBucket.Stats().KeyN
		if keyCount > 0 {
			results = make([]*UserTraffic, 0, keyCount)
		}

		c := usersBucket.Cursor()
		for k, v := c.First(); k != nil; k, v = c.Next() {
			if v == nil {
				continue
			}
			traffic, err := decodeUserTraffic(v)
			if err != nil {
				return fmt.Errorf("failed to decode user traffic for %s: %w", string(k), err)
			}
			traffic.Email = string(k)
			results = append(results, traffic)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return results, nil
}

// GetTrafficHistory returns time series snapshots for a given user within [startTime, endTime] (inclusive).
func (s *BoltStorage) GetTrafficHistory(email string, startTime, endTime int64) ([]TrafficSnapshot, error) {
	if s == nil || s.isClosed() {
		return nil, ErrDatabaseClosed
	}
	if strings.TrimSpace(email) == "" {
		return nil, ErrEmptyEmail
	}
	if strings.Contains(email, string(keyDelimiter)) {
		return nil, fmt.Errorf("%w: email contains invalid null byte", domain.ErrInvalidInput)
	}
	if startTime < 0 || endTime < 0 || startTime > endTime {
		return nil, fmt.Errorf("%w: startTime (%d) and endTime (%d) must be non-negative with startTime <= endTime", ErrInvalidTimeRange, startTime, endTime)
	}

	results := make([]TrafficSnapshot, 0)
	err := s.db.View(func(tx *bbolt.Tx) error {
		historyBucket := tx.Bucket([]byte(BucketTrafficHistory))
		if historyBucket == nil {
			return fmt.Errorf("bucket %s not found", BucketTrafficHistory)
		}

		prefix := makeHistoryPrefix(email)
		startKey := makeHistoryKey(email, startTime)

		c := historyBucket.Cursor()
		for k, v := c.Seek(startKey); k != nil; k, v = c.Next() {
			if !bytes.HasPrefix(k, prefix) {
				break
			}
			if v == nil || len(k) != len(prefix)+8 {
				continue
			}
			ts := extractTimestampFromKey(k, len(prefix))
			if ts > endTime {
				break
			}

			snap, err := decodeTrafficSnapshot(v)
			if err != nil {
				return fmt.Errorf("failed to decode snapshot at %d: %w", ts, err)
			}
			snap.Email = email
			snap.Timestamp = ts
			results = append(results, *snap)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return results, nil
}

// ResetUserTraffic resets cumulative traffic (Uplink and Downlink) for a user to zero.
func (s *BoltStorage) ResetUserTraffic(email string) error {
	if s == nil || s.isClosed() {
		return ErrDatabaseClosed
	}
	if strings.TrimSpace(email) == "" {
		return ErrEmptyEmail
	}
	if strings.Contains(email, string(keyDelimiter)) {
		return fmt.Errorf("%w: email contains invalid null byte", domain.ErrInvalidInput)
	}

	return s.db.Update(func(tx *bbolt.Tx) error {
		usersBucket := tx.Bucket([]byte(BucketUsersTraffic))
		if usersBucket == nil {
			return fmt.Errorf("bucket %s not found", BucketUsersTraffic)
		}

		userKey := []byte(email)
		data := usersBucket.Get(userKey)
		if data == nil {
			return ErrUserNotFound
		}

		userTraffic, err := decodeUserTraffic(data)
		if err != nil {
			return fmt.Errorf("failed to decode user traffic for %s: %w", email, err)
		}

		userTraffic.Email = email
		userTraffic.Uplink = 0
		userTraffic.Downlink = 0

		userBytes, err := encodeUserTraffic(userTraffic, s.opts.Serializer)
		if err != nil {
			return fmt.Errorf("failed to encode user traffic for %s: %w", email, err)
		}
		return usersBucket.Put(userKey, userBytes)
	})
}

// DeleteUserTraffic removes a user's cumulative traffic and all historical records.
func (s *BoltStorage) DeleteUserTraffic(email string) error {
	if s == nil || s.isClosed() {
		return ErrDatabaseClosed
	}
	if strings.TrimSpace(email) == "" {
		return ErrEmptyEmail
	}
	if strings.Contains(email, string(keyDelimiter)) {
		return fmt.Errorf("%w: email contains invalid null byte", domain.ErrInvalidInput)
	}

	return s.db.Update(func(tx *bbolt.Tx) error {
		usersBucket := tx.Bucket([]byte(BucketUsersTraffic))
		if usersBucket == nil {
			return fmt.Errorf("bucket %s not found", BucketUsersTraffic)
		}
		historyBucket := tx.Bucket([]byte(BucketTrafficHistory))
		if historyBucket == nil {
			return fmt.Errorf("bucket %s not found", BucketTrafficHistory)
		}

		userKey := []byte(email)
		if usersBucket.Get(userKey) == nil {
			return ErrUserNotFound
		}
		if err := usersBucket.Delete(userKey); err != nil {
			return fmt.Errorf("failed to delete user traffic for %s: %w", email, err)
		}

		// Collect all history keys for this user, then delete to avoid cursor mutation issues
		prefix := makeHistoryPrefix(email)
		var keysToDelete [][]byte
		c := historyBucket.Cursor()
		for k, _ := c.Seek(prefix); k != nil && bytes.HasPrefix(k, prefix); k, _ = c.Next() {
			keyCopy := make([]byte, len(k))
			copy(keyCopy, k)
			keysToDelete = append(keysToDelete, keyCopy)
		}
		for _, k := range keysToDelete {
			if err := historyBucket.Delete(k); err != nil {
				return fmt.Errorf("failed to delete history for %s: %w", email, err)
			}
		}

		return nil
	})
}
