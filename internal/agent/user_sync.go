package agent

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
)

// UserController abstracts local Xray gRPC dynamic user hot-swap operations.
type UserController interface {
	AddUser(ctx context.Context, user RemoteUser) error
	RemoveUser(ctx context.Context, inboundTag, email string) error
}

// UserSyncer performs differential AddUser / RemoveUser updates against Xray without restarting it.
type UserSyncer struct {
	mu      sync.Mutex
	ctrl    UserController
	applied map[string]RemoteUser
}

// NewUserSyncer creates a UserSyncer backed by ctrl.
func NewUserSyncer(ctrl UserController) (*UserSyncer, error) {
	if ctrl == nil {
		return nil, errors.New("user controller is required")
	}
	return &UserSyncer{
		ctrl:    ctrl,
		applied: make(map[string]RemoteUser),
	}, nil
}

func userSyncKey(inboundTag, email string) string {
	return inboundTag + "\x00" + email
}

// ResetBaseline replaces the in-memory applied user map after a full Xray config reload.
func (s *UserSyncer) ResetBaseline(users []RemoteUser) {
	s.mu.Lock()
	defer s.mu.Unlock()

	next := make(map[string]RemoteUser, len(users))
	for _, u := range users {
		if u.InboundTag == "" || u.Email == "" {
			continue
		}
		next[userSyncKey(u.InboundTag, u.Email)] = u
	}
	s.applied = next
}

// Reconcile diffs desired against the applied baseline and executes only the necessary
// RemoveUser and AddUser gRPC calls.
func (s *UserSyncer) Reconcile(ctx context.Context, desired []RemoteUser) (added int, removed int, err error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	desiredMap := make(map[string]RemoteUser, len(desired))
	for _, u := range desired {
		if u.InboundTag == "" || u.Email == "" {
			continue
		}
		desiredMap[userSyncKey(u.InboundTag, u.Email)] = u
	}

	var toRemove []RemoteUser
	var toAdd []RemoteUser

	for k, cur := range s.applied {
		want, exists := desiredMap[k]
		if !exists {
			toRemove = append(toRemove, cur)
			continue
		}
		if cur.UUID != want.UUID || cur.Flow != want.Flow || !strings.EqualFold(cur.Protocol, want.Protocol) {
			toRemove = append(toRemove, cur)
			toAdd = append(toAdd, want)
		}
	}

	for k, want := range desiredMap {
		if _, exists := s.applied[k]; !exists {
			toAdd = append(toAdd, want)
		}
	}

	var errs []error

	for _, u := range toRemove {
		key := userSyncKey(u.InboundTag, u.Email)
		if rmErr := s.ctrl.RemoveUser(ctx, u.InboundTag, u.Email); rmErr != nil {
			if !isIgnorableXrayError(rmErr) {
				errs = append(errs, fmt.Errorf("remove user %s from %s: %w", u.Email, u.InboundTag, rmErr))
				continue
			}
		}
		delete(s.applied, key)
		removed++
	}

	for _, u := range toAdd {
		key := userSyncKey(u.InboundTag, u.Email)
		if addErr := s.ctrl.AddUser(ctx, u); addErr != nil {
			if !isIgnorableXrayError(addErr) {
				errs = append(errs, fmt.Errorf("add user %s to %s: %w", u.Email, u.InboundTag, addErr))
				continue
			}
		}
		s.applied[key] = u
		added++
	}

	return added, removed, errors.Join(errs...)
}

func isIgnorableXrayError(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "not found") ||
		strings.Contains(msg, "not exist") ||
		strings.Contains(msg, "handler not found") ||
		strings.Contains(msg, "failed to get handler") ||
		strings.Contains(msg, "not a usermanager") ||
		strings.Contains(msg, "does not support dynamic account operations")
}
