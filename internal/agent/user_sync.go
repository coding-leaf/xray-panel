package agent

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sort"
	"strings"
	"sync"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// ErrInboundHandlerMissing indicates that Xray runtime is missing an expected inbound handler.
var ErrInboundHandlerMissing = errors.New("xray inbound handler not found")

// UserController abstracts local Xray gRPC dynamic user hot-swap operations.
type UserController interface {
	AddUser(ctx context.Context, user RemoteUser) error
	RemoveUser(ctx context.Context, inboundTag, email string) error
}

// RuntimeUserInspector is an optional capability on UserController for querying live inbound users.
type RuntimeUserInspector interface {
	GetInboundUsers(ctx context.Context, inboundTag string) ([]RemoteUser, error)
}

// SysUptimeProvider is an optional capability for detecting Xray restarts via gRPC GetSysStats uptime.
type SysUptimeProvider interface {
	GetSysUptime(ctx context.Context) (uint32, error)
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
	return s.reconcileLocked(ctx, desired)
}

func (s *UserSyncer) reconcileLocked(ctx context.Context, desired []RemoteUser) (added int, removed int, err error) {
	desiredMap := buildDesiredUserMap(desired)

	var toRemove []RemoteUser
	var toAdd []RemoteUser

	for k, cur := range s.applied {
		want, exists := desiredMap[k]
		if !exists {
			toRemove = append(toRemove, cur)
			continue
		}
		if userRuntimeChanged(cur, want) {
			toRemove = append(toRemove, cur)
			toAdd = append(toAdd, want)
		}
	}

	for k, want := range desiredMap {
		if _, exists := s.applied[k]; !exists {
			toAdd = append(toAdd, want)
		}
	}

	return s.applyDiffLocked(ctx, toRemove, toAdd)
}

// ReconcileWithRuntime inspects live Xray inbound users via GetInboundUsers (when supported),
// preserves static clients, diffs against desired, and applies RemoveUser/AddUser to converge runtime.
// When GetInboundUsers is unimplemented on older Xray builds or unsupported by ctrl, it falls back to Reconcile.
func (s *UserSyncer) ReconcileWithRuntime(
	ctx context.Context,
	desired []RemoteUser,
	dynamicTags []string,
	staticEmails map[string]struct{},
) (added int, removed int, err error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	inspector, ok := s.ctrl.(RuntimeUserInspector)
	if !ok {
		return s.reconcileLocked(ctx, desired)
	}

	tagSet := make(map[string]struct{}, len(dynamicTags)+len(desired))
	for _, tag := range dynamicTags {
		t := strings.TrimSpace(tag)
		if t == "" || t == "api" {
			continue
		}
		tagSet[t] = struct{}{}
	}
	for _, u := range desired {
		t := strings.TrimSpace(u.InboundTag)
		if t == "" || t == "api" {
			continue
		}
		tagSet[t] = struct{}{}
	}

	if len(tagSet) == 0 {
		s.applied = make(map[string]RemoteUser)
		return 0, 0, nil
	}

	tags := make([]string, 0, len(tagSet))
	for tag := range tagSet {
		tags = append(tags, tag)
	}
	sort.Strings(tags)

	liveMap := make(map[string]RemoteUser)
	for _, tag := range tags {
		runtimeUsers, getErr := inspector.GetInboundUsers(ctx, tag)
		if getErr != nil {
			if isUnimplementedError(getErr) {
				slog.Warn("Xray GetInboundUsers is unimplemented; falling back to baseline diff", slog.String("tag", tag))
				return s.reconcileLocked(ctx, desired)
			}
			if isHandlerNotFoundError(getErr) {
				return 0, 0, fmt.Errorf("%w: inspect inbound %s: %w", ErrInboundHandlerMissing, tag, getErr)
			}
			if isNonUserManagerError(getErr) {
				continue
			}
			return 0, 0, fmt.Errorf("inspect inbound %s users: %w", tag, getErr)
		}

		for _, ru := range runtimeUsers {
			email := strings.TrimSpace(ru.Email)
			if email == "" || isStaticClientEmail(email, staticEmails) {
				continue
			}
			if ru.InboundTag == "" {
				ru.InboundTag = tag
			}
			ru.Email = email
			liveMap[userSyncKey(ru.InboundTag, ru.Email)] = ru
		}
	}

	desiredMap := buildDesiredUserMap(desired)

	var toRemove []RemoteUser
	var toAdd []RemoteUser

	for k, cur := range liveMap {
		want, exists := desiredMap[k]
		if !exists {
			toRemove = append(toRemove, cur)
			continue
		}
		if userRuntimeChanged(cur, want) {
			toRemove = append(toRemove, cur)
			toAdd = append(toAdd, want)
		}
	}

	for k, want := range desiredMap {
		if _, exists := liveMap[k]; !exists {
			toAdd = append(toAdd, want)
		}
	}

	s.applied = liveMap
	return s.applyDiffLocked(ctx, toRemove, toAdd)
}

func (s *UserSyncer) applyDiffLocked(ctx context.Context, toRemove, toAdd []RemoteUser) (added int, removed int, err error) {
	var errs []error

	for _, u := range toRemove {
		key := userSyncKey(u.InboundTag, u.Email)
		if rmErr := s.ctrl.RemoveUser(ctx, u.InboundTag, u.Email); rmErr != nil {
			if !isIgnorableRemoveUserError(rmErr) {
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
			if isHandlerNotFoundError(addErr) {
				errs = append(errs, fmt.Errorf("%w: add user %s to %s: %w", ErrInboundHandlerMissing, u.Email, u.InboundTag, addErr))
				continue
			}
			if !isIgnorableAddUserError(addErr) {
				errs = append(errs, fmt.Errorf("add user %s to %s: %w", u.Email, u.InboundTag, addErr))
				continue
			}
		}
		s.applied[key] = u
		added++
	}

	return added, removed, errors.Join(errs...)
}

func buildDesiredUserMap(desired []RemoteUser) map[string]RemoteUser {
	desiredMap := make(map[string]RemoteUser, len(desired))
	for _, u := range desired {
		if u.InboundTag == "" || u.Email == "" {
			continue
		}
		desiredMap[userSyncKey(u.InboundTag, u.Email)] = u
	}
	return desiredMap
}

func isStaticClientEmail(email string, staticEmails map[string]struct{}) bool {
	if isBuiltinStaticClientEmail(email) {
		return true
	}
	if len(staticEmails) > 0 {
		_, ok := staticEmails[email]
		return ok
	}
	return false
}

func userRuntimeChanged(cur, want RemoteUser) bool {
	if cur.UUID != want.UUID {
		return true
	}
	curProto := canonicalRuntimeProtocol(cur.Protocol, cur.Method)
	wantProto := canonicalRuntimeProtocol(want.Protocol, want.Method)
	if curProto != "" && curProto != wantProto {
		return true
	}
	switch wantProto {
	case "vless":
		return cur.Flow != want.Flow
	case "shadowsocks":
		return canonicalShadowsocksMethod(cur.Method) != canonicalShadowsocksMethod(want.Method)
	default:
		// vmess, trojan, shadowsocks-2022 compare UUID/password/key only.
		return false
	}
}

func canonicalRuntimeProtocol(protocol, method string) string {
	p := strings.ToLower(strings.TrimSpace(protocol))
	m := strings.ToLower(strings.TrimSpace(method))
	switch p {
	case "ss", "shadowsocks":
		if strings.HasPrefix(m, "2022-blake3-") {
			return "shadowsocks-2022"
		}
		return "shadowsocks"
	case "ss-2022", "ss2022", "shadowsocks-2022":
		return "shadowsocks-2022"
	default:
		return p
	}
}

func canonicalShadowsocksMethod(method string) string {
	m := strings.ToLower(strings.TrimSpace(method))
	switch m {
	case "", "aes-128-gcm":
		return "aes-128-gcm"
	case "chacha20-ietf-poly1305", "chacha20-poly1305":
		return "chacha20-poly1305"
	case "xchacha20-ietf-poly1305", "xchacha20-poly1305":
		return "xchacha20-poly1305"
	default:
		return m
	}
}

func isHandlerNotFoundError(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "handler not found") || strings.Contains(msg, "failed to get handler")
}

func isNonUserManagerError(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "not a usermanager") || strings.Contains(msg, "does not support dynamic account operations")
}

func isUnimplementedError(err error) bool {
	if err == nil {
		return false
	}
	type grpcStatus interface {
		GRPCStatus() *status.Status
	}
	var gs grpcStatus
	if errors.As(err, &gs) && gs.GRPCStatus().Code() == codes.Unimplemented {
		return true
	}
	return status.Code(err) == codes.Unimplemented
}

func isIgnorableAddUserError(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "already exists") || isNonUserManagerError(err)
}

func isIgnorableRemoveUserError(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "not found") ||
		strings.Contains(msg, "not exist") ||
		isHandlerNotFoundError(err) ||
		isNonUserManagerError(err)
}
