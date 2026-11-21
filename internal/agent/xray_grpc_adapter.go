package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"panel/internal/adapter/xray"
	"panel/internal/domain"
)

// TrafficCollector queries local Xray traffic counters and returns a non-empty delta batch.
type TrafficCollector interface {
	CollectTrafficStats(ctx context.Context) (*TrafficBatch, error)
}

// XrayGRPCAdapter adapts internal/adapter/xray.GRPCClient to the narrow interfaces
// required by ConfigManager, UserSyncer, and Runner.
type XrayGRPCAdapter struct {
	client *xray.GRPCClient
}

// NewXrayGRPCAdapter wraps a *xray.GRPCClient.
func NewXrayGRPCAdapter(client *xray.GRPCClient) (*XrayGRPCAdapter, error) {
	if client == nil {
		return nil, errors.New("xray grpc client is required")
	}
	return &XrayGRPCAdapter{client: client}, nil
}

// Probe verifies that the local Xray gRPC server responds to stats queries.
func (a *XrayGRPCAdapter) Probe(ctx context.Context) error {
	_, err := a.client.QueryTrafficStats(ctx, false)
	return err
}

// GetSysUptime queries Xray process uptime in seconds via StatsService.GetSysStats.
func (a *XrayGRPCAdapter) GetSysUptime(ctx context.Context) (uint32, error) {
	return a.client.GetSysUptime(ctx)
}

// GetInboundUsers queries live dynamic and static users currently registered on inboundTag.
func (a *XrayGRPCAdapter) GetInboundUsers(ctx context.Context, inboundTag string) ([]RemoteUser, error) {
	runtimeUsers, err := a.client.GetInboundUsers(ctx, inboundTag)
	if err != nil {
		return nil, err
	}
	out := make([]RemoteUser, 0, len(runtimeUsers))
	for _, ru := range runtimeUsers {
		out = append(out, RemoteUser{
			InboundTag: ru.InboundTag,
			Protocol:   ru.Protocol,
			Email:      ru.Email,
			UUID:       ru.UUID,
			Flow:       ru.Flow,
			Method:     ru.Method,
		})
	}
	return out, nil
}

// AddUser adds a user credential to the target Xray inbound via gRPC.
func (a *XrayGRPCAdapter) AddUser(ctx context.Context, user RemoteUser) error {
	streamSettings := ""
	if user.Flow != "" {
		streamSettings = `{"network":"tcp","security":"tls"}`
	}
	settingsJSON := ""
	if method := strings.TrimSpace(user.Method); method != "" {
		raw, err := json.Marshal(struct {
			Method string `json:"method"`
		}{Method: method})
		if err != nil {
			return fmt.Errorf("marshal shadowsocks method settings failed: %w", err)
		}
		settingsJSON = string(raw)
	}
	ib := &domain.Inbound{
		Tag:            user.InboundTag,
		Protocol:       user.Protocol,
		SettingsJSON:   settingsJSON,
		StreamSettings: streamSettings,
	}
	u := &domain.User{
		Email: user.Email,
		UUID:  user.UUID,
		Flow:  user.Flow,
	}
	return a.client.AddUser(ctx, ib, u)
}

// RemoveUser removes a user credential from the target Xray inbound via gRPC.
func (a *XrayGRPCAdapter) RemoveUser(ctx context.Context, inboundTag, email string) error {
	return a.client.RemoveUser(ctx, inboundTag, email)
}

// CollectTrafficStats queries and resets Xray traffic stats, returning nil if all deltas are zero.
func (a *XrayGRPCAdapter) CollectTrafficStats(ctx context.Context) (*TrafficBatch, error) {
	stats, err := a.client.QueryTrafficStats(ctx, true)
	if err != nil {
		return nil, err
	}

	userMap := make(map[string]*UserTrafficDelta)
	inboundMap := make(map[string]*InboundTrafficDelta)

	for _, st := range stats {
		if st.Value <= 0 {
			continue
		}
		switch st.Type {
		case domain.TrafficStatTypeUser:
			entry, ok := userMap[st.Tag]
			if !ok {
				entry = &UserTrafficDelta{Email: st.Tag}
				userMap[st.Tag] = entry
			}
			if st.IsUplink {
				entry.UpBytes += st.Value
			} else {
				entry.DownBytes += st.Value
			}
		case domain.TrafficStatTypeInbound:
			if st.Tag == "api" {
				continue
			}
			entry, ok := inboundMap[st.Tag]
			if !ok {
				entry = &InboundTrafficDelta{Tag: st.Tag}
				inboundMap[st.Tag] = entry
			}
			if st.IsUplink {
				entry.UpBytes += st.Value
			} else {
				entry.DownBytes += st.Value
			}
		}
	}

	if len(userMap) == 0 && len(inboundMap) == 0 {
		return nil, nil
	}

	batch := &TrafficBatch{
		BatchID: NewBatchID(),
	}
	for _, u := range userMap {
		batch.Users = append(batch.Users, *u)
	}
	for _, ib := range inboundMap {
		batch.Inbounds = append(batch.Inbounds, *ib)
	}
	return batch, nil
}
