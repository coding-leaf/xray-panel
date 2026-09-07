package xray

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"go.etcd.io/bbolt"
)

const (
	// BucketInboundUsers stores serialized InboundUser records keyed by "inboundTag:email".
	BucketInboundUsers = "inbound_users"

	// BucketManagedInbounds tracks all inbound tags that are managed by the runtime system.
	BucketManagedInbounds = "managed_inbounds"
)

var (
	ErrUserNotFound = errors.New("inbound user not found")
)

// InboundUser represents a client user authorized on a specific Xray inbound.
type InboundUser struct {
	InboundTag string    `json:"inbound_tag"`
	Email      string    `json:"email"`
	UUID       string    `json:"uuid"`
	Flow       string    `json:"flow,omitempty"`
	Protocol   string    `json:"protocol,omitempty"` // Defaults to "vless" if empty
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}

func userStorageKey(inboundTag, email string) []byte {
	return []byte(inboundTag + "\x00" + email)
}

// InitStorage ensures all necessary BoltDB buckets are initialized.
func InitStorage(db *bbolt.DB) error {
	if db == nil {
		return ErrStorageNotAvailable
	}
	return db.Update(func(tx *bbolt.Tx) error {
		if _, err := tx.CreateBucketIfNotExists([]byte(BucketInboundUsers)); err != nil {
			return fmt.Errorf("create bucket %s failed: %w", BucketInboundUsers, err)
		}
		if _, err := tx.CreateBucketIfNotExists([]byte(BucketManagedInbounds)); err != nil {
			return fmt.Errorf("create bucket %s failed: %w", BucketManagedInbounds, err)
		}
		return nil
	})
}

func (c *XrayClient) initStorage() error {
	return InitStorage(c.db)
}

// SaveUser persists or updates an InboundUser record in BoltDB and marks the inbound tag as managed.
func SaveUser(db *bbolt.DB, u *InboundUser) error {
	if db == nil {
		return ErrStorageNotAvailable
	}
	if u == nil {
		return fmt.Errorf("%w: user cannot be nil", ErrInvalidParameter)
	}

	tag := strings.TrimSpace(u.InboundTag)
	email := strings.TrimSpace(u.Email)
	uuid := strings.TrimSpace(u.UUID)
	if tag == "" || email == "" || uuid == "" {
		return fmt.Errorf("%w: user inboundTag, email, and uuid are required", ErrInvalidParameter)
	}

	return db.Update(func(tx *bbolt.Tx) error {
		b, err := tx.CreateBucketIfNotExists([]byte(BucketInboundUsers))
		if err != nil {
			return fmt.Errorf("create bucket %s failed: %w", BucketInboundUsers, err)
		}

		key := userStorageKey(tag, email)
		now := time.Now()

		uRecord := *u
		uRecord.InboundTag = tag
		uRecord.Email = email
		uRecord.UUID = uuid

		// Preserve existing CreatedAt timestamp if present
		if existingRaw := b.Get(key); len(existingRaw) > 0 {
			var existing InboundUser
			if err := json.Unmarshal(existingRaw, &existing); err == nil && !existing.CreatedAt.IsZero() {
				uRecord.CreatedAt = existing.CreatedAt
			}
		}
		if uRecord.CreatedAt.IsZero() {
			uRecord.CreatedAt = now
		}
		uRecord.UpdatedAt = now

		data, err := json.Marshal(&uRecord)
		if err != nil {
			return fmt.Errorf("marshal inbound user failed: %w", err)
		}

		if err := b.Put(key, data); err != nil {
			return fmt.Errorf("put user in bolt failed: %w", err)
		}

		// Mark inbound tag as managed
		mb, err := tx.CreateBucketIfNotExists([]byte(BucketManagedInbounds))
		if err == nil && mb != nil {
			_ = mb.Put([]byte(tag), []byte("1"))
		}

		// Also update caller's timestamps safely
		u.CreatedAt = uRecord.CreatedAt
		u.UpdatedAt = uRecord.UpdatedAt

		return nil
	})
}

// RemoveUser deletes an InboundUser record from BoltDB.
func RemoveUser(db *bbolt.DB, inboundTag, email string) error {
	if db == nil {
		return ErrStorageNotAvailable
	}
	inboundTag = strings.TrimSpace(inboundTag)
	email = strings.TrimSpace(email)
	if inboundTag == "" || email == "" {
		return fmt.Errorf("%w: inboundTag and email are required", ErrInvalidParameter)
	}

	return db.Update(func(tx *bbolt.Tx) error {
		b := tx.Bucket([]byte(BucketInboundUsers))
		if b == nil {
			return nil // Bucket does not exist yet; user is already absent
		}

		key := userStorageKey(inboundTag, email)
		return b.Delete(key)
	})
}

// GetUser retrieves a specific InboundUser from BoltDB.
func GetUser(db *bbolt.DB, inboundTag, email string) (*InboundUser, error) {
	if db == nil {
		return nil, ErrStorageNotAvailable
	}
	inboundTag = strings.TrimSpace(inboundTag)
	email = strings.TrimSpace(email)
	if inboundTag == "" || email == "" {
		return nil, fmt.Errorf("%w: inboundTag and email are required", ErrInvalidParameter)
	}

	var user *InboundUser
	err := db.View(func(tx *bbolt.Tx) error {
		b := tx.Bucket([]byte(BucketInboundUsers))
		if b == nil {
			return ErrUserNotFound
		}

		key := userStorageKey(inboundTag, email)
		val := b.Get(key)
		if val == nil {
			return ErrUserNotFound
		}

		var u InboundUser
		if err := json.Unmarshal(val, &u); err != nil {
			return fmt.Errorf("unmarshal user failed: %w", err)
		}
		user = &u
		return nil
	})
	return user, err
}

// ListUsers retrieves all users for a given inboundTag, or all users if inboundTag is not specified.
func ListUsers(db *bbolt.DB, inboundTag ...string) ([]*InboundUser, error) {
	if db == nil {
		return nil, ErrStorageNotAvailable
	}

	targetTag := ""
	if len(inboundTag) > 0 {
		targetTag = strings.TrimSpace(inboundTag[0])
	}

	var list []*InboundUser
	err := db.View(func(tx *bbolt.Tx) error {
		b := tx.Bucket([]byte(BucketInboundUsers))
		if b == nil {
			return nil
		}

		c := b.Cursor()
		if targetTag != "" {
			prefix := []byte(targetTag + "\x00")
			for k, v := c.Seek(prefix); k != nil && bytes.HasPrefix(k, prefix); k, v = c.Next() {
				var u InboundUser
				if err := json.Unmarshal(v, &u); err == nil {
					list = append(list, &u)
				}
			}
		} else {
			for k, v := c.First(); k != nil; k, v = c.Next() {
				var u InboundUser
				if err := json.Unmarshal(v, &u); err == nil {
					list = append(list, &u)
				}
			}
		}
		return nil
	})
	return list, err
}

// GetAllUsers retrieves all InboundUser records across all inbounds.
func GetAllUsers(db *bbolt.DB) ([]*InboundUser, error) {
	return ListUsers(db)
}

// GetManagedInbounds returns a set of all inbound tags that have been recorded as managed.
func GetManagedInbounds(db *bbolt.DB) (map[string]bool, error) {
	if db == nil {
		return nil, ErrStorageNotAvailable
	}

	managed := make(map[string]bool)
	err := db.View(func(tx *bbolt.Tx) error {
		mb := tx.Bucket([]byte(BucketManagedInbounds))
		if mb == nil {
			return nil
		}
		c := mb.Cursor()
		for k, _ := c.First(); k != nil; k, _ = c.Next() {
			managed[string(k)] = true
		}
		return nil
	})
	return managed, err
}

func (c *XrayClient) saveUserToDB(u *InboundUser) error {
	return SaveUser(c.db, u)
}

func (c *XrayClient) removeUserFromDB(inboundTag, email string) error {
	return RemoveUser(c.db, inboundTag, email)
}

// GetInboundUser retrieves an authorized user by inboundTag and email from the local BoltDB store.
func (c *XrayClient) GetInboundUser(inboundTag, email string) (*InboundUser, error) {
	return GetUser(c.db, inboundTag, email)
}

// ListInboundUsers returns users filtered by inboundTag (or all if not provided).
func (c *XrayClient) ListInboundUsers(inboundTag ...string) ([]*InboundUser, error) {
	return ListUsers(c.db, inboundTag...)
}

// GetAllInboundUsers returns all users in the local BoltDB store.
func (c *XrayClient) GetAllInboundUsers() ([]*InboundUser, error) {
	return GetAllUsers(c.db)
}
