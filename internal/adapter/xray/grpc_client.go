package xray

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"panel/internal/adapter/xray/proto"
	"panel/internal/domain"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/keepalive"
)

type GRPCClient struct {
	addr       string
	mu         sync.RWMutex
	conn       *grpc.ClientConn
	handlerCli proto.HandlerServiceClient
	statsCli   proto.StatsServiceClient
}

func NewGRPCClient(addr string) *GRPCClient {
	if addr == "" {
		addr = "127.0.0.1:8080"
	}
	return &GRPCClient{
		addr: addr,
	}
}

func (c *GRPCClient) getClients(ctx context.Context) (proto.HandlerServiceClient, proto.StatsServiceClient, error) {
	if err := ctx.Err(); err != nil {
		return nil, nil, err
	}

	c.mu.RLock()
	if c.conn != nil && c.handlerCli != nil && c.statsCli != nil {
		h, s := c.handlerCli, c.statsCli
		c.mu.RUnlock()
		return h, s, nil
	}
	c.mu.RUnlock()

	c.mu.Lock()
	defer c.mu.Unlock()

	if c.conn != nil && c.handlerCli != nil && c.statsCli != nil {
		return c.handlerCli, c.statsCli, nil
	}

	if err := ctx.Err(); err != nil {
		return nil, nil, err
	}

	conn, err := grpc.NewClient(
		c.addr,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithKeepaliveParams(keepalive.ClientParameters{
			Time:    30 * time.Second,
			Timeout: 5 * time.Second,
		}),
	)
	if err != nil {
		return nil, nil, fmt.Errorf("%w: connect gRPC at %s failed: %v", domain.ErrXrayUnavailable, c.addr, err)
	}

	c.conn = conn
	c.handlerCli = proto.NewHandlerServiceClient(conn)
	c.statsCli = proto.NewStatsServiceClient(conn)
	return c.handlerCli, c.statsCli, nil
}

func ensureRPCTimeout(ctx context.Context) (context.Context, context.CancelFunc) {
	if _, ok := ctx.Deadline(); ok {
		return ctx, func() {}
	}
	return context.WithTimeout(ctx, 5*time.Second)
}

func (c *GRPCClient) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.conn != nil {
		err := c.conn.Close()
		c.conn = nil
		c.handlerCli = nil
		c.statsCli = nil
		return err
	}
	return nil
}

// AddUser 向指定 Inbound 添加用户
func (c *GRPCClient) AddUser(ctx context.Context, inbound *domain.Inbound, user *domain.User) error {
	handlerCli, _, err := c.getClients(ctx)
	if err != nil {
		return err
	}

	accountMsg, err := BuildAccountMessage(inbound, user)
	if err != nil {
		return err
	}

	protoUser := &proto.User{
		Level:   0,
		Email:   user.Email,
		Account: accountMsg,
	}

	opMsg, err := proto.ToTypedMessage(&proto.AddUserOperation{
		User: protoUser,
	})
	if err != nil {
		return err
	}

	req := &proto.AlterInboundRequest{
		Tag:       inbound.Tag,
		Operation: opMsg,
	}

	rpcCtx, cancel := ensureRPCTimeout(ctx)
	defer cancel()

	_, err = handlerCli.AlterInbound(rpcCtx, req)
	if err != nil {
		return fmt.Errorf("xray alter inbound add user failed: %w", err)
	}
	return nil
}

// RemoveUser 从指定 Inbound 移除用户
func (c *GRPCClient) RemoveUser(ctx context.Context, inboundTag string, email string) error {
	handlerCli, _, err := c.getClients(ctx)
	if err != nil {
		return err
	}

	opMsg, err := proto.ToTypedMessage(&proto.RemoveUserOperation{
		Email: email,
	})
	if err != nil {
		return err
	}

	req := &proto.AlterInboundRequest{
		Tag:       inboundTag,
		Operation: opMsg,
	}

	rpcCtx, cancel := ensureRPCTimeout(ctx)
	defer cancel()

	_, err = handlerCli.AlterInbound(rpcCtx, req)
	if err != nil {
		return fmt.Errorf("xray alter inbound remove user failed: %w", err)
	}
	return nil
}

// QueryTrafficStats 查询流量统计
func (c *GRPCClient) QueryTrafficStats(ctx context.Context, reset bool) ([]domain.TrafficStat, error) {
	_, statsCli, err := c.getClients(ctx)
	if err != nil {
		return nil, err
	}

	req := &proto.QueryStatsRequest{
		Reset_: reset,
	}

	rpcCtx, cancel := ensureRPCTimeout(ctx)
	defer cancel()

	resp, err := statsCli.QueryStats(rpcCtx, req)
	if err != nil {
		return nil, fmt.Errorf("xray query stats failed: %w", err)
	}

	var results []domain.TrafficStat
	for _, stat := range resp.GetStat() {
		name := stat.GetName()
		value := stat.GetValue()
		if value == 0 {
			continue
		}

		parts := strings.Split(name, ">>>")
		if len(parts) < 4 {
			continue
		}

		statType := parts[0]
		tagOrEmail := parts[1]
		direction := parts[3]
		isUplink := (direction == "uplink")

		var domainType domain.TrafficStatType
		switch statType {
		case "user":
			domainType = domain.TrafficStatTypeUser
		case "inbound":
			domainType = domain.TrafficStatTypeInbound
		case "outbound":
			domainType = domain.TrafficStatTypeOutbound
		default:
			continue
		}

		results = append(results, domain.TrafficStat{
			Type:     domainType,
			Tag:      tagOrEmail,
			IsUplink: isUplink,
			Value:    value,
		})
	}
	return results, nil
}

// RuntimeInboundUser represents a live user decoded from Xray HandlerService.GetInboundUsers.
type RuntimeInboundUser struct {
	InboundTag string
	Email      string
	Protocol   string // "vless" | "vmess" | "trojan" | "shadowsocks" | "shadowsocks-2022"
	UUID       string // VLESS/VMess id, Trojan/SS password, SS2022 key
	Flow       string // VLESS flow
	Method     string // Shadowsocks cipher (lowercase, e.g. "aes-256-gcm"); empty for SS2022
}

// DecodeAccountMessage decodes an Xray TypedMessage account payload into normalized fields.
func DecodeAccountMessage(account *proto.TypedMessage) (RuntimeInboundUser, error) {
	if account == nil {
		return RuntimeInboundUser{}, errors.New("account typed message is nil")
	}
	inst, err := account.GetInstance()
	if err != nil {
		return RuntimeInboundUser{}, err
	}
	switch acc := inst.(type) {
	case *proto.VLESSAccount:
		return RuntimeInboundUser{
			Protocol: "vless",
			UUID:     acc.GetId(),
			Flow:     acc.GetFlow(),
		}, nil
	case *proto.VMessAccount:
		return RuntimeInboundUser{
			Protocol: "vmess",
			UUID:     acc.GetId(),
		}, nil
	case *proto.TrojanAccount:
		return RuntimeInboundUser{
			Protocol: "trojan",
			UUID:     acc.GetPassword(),
		}, nil
	case *proto.ShadowsocksAccount:
		return RuntimeInboundUser{
			Protocol: "shadowsocks",
			UUID:     acc.GetPassword(),
			Method:   cipherTypeToMethod(acc.GetCipherType()),
		}, nil
	case *proto.Shadowsocks2022Account:
		return RuntimeInboundUser{
			Protocol: "shadowsocks-2022",
			UUID:     acc.GetKey(),
		}, nil
	default:
		return RuntimeInboundUser{}, fmt.Errorf("unexpected account message type %T", inst)
	}
}

func cipherTypeToMethod(ct proto.CipherType) string {
	switch ct {
	case proto.CipherType_AES_128_GCM:
		return "aes-128-gcm"
	case proto.CipherType_AES_256_GCM:
		return "aes-256-gcm"
	case proto.CipherType_CHACHA20_POLY1305:
		return "chacha20-poly1305"
	case proto.CipherType_XCHACHA20_POLY1305:
		return "xchacha20-poly1305"
	case proto.CipherType_NONE:
		return "none"
	default:
		return ""
	}
}

// GetInboundUsers queries live users registered on the specified Xray inbound tag.
func (c *GRPCClient) GetInboundUsers(ctx context.Context, inboundTag string) ([]RuntimeInboundUser, error) {
	handlerCli, _, err := c.getClients(ctx)
	if err != nil {
		return nil, err
	}

	rpcCtx, cancel := ensureRPCTimeout(ctx)
	defer cancel()

	resp, err := handlerCli.GetInboundUsers(rpcCtx, &proto.GetInboundUserRequest{
		Tag: inboundTag,
	})
	if err != nil {
		return nil, fmt.Errorf("xray get inbound users for %s failed: %w", inboundTag, err)
	}

	users := make([]RuntimeInboundUser, 0, len(resp.GetUsers()))
	for _, u := range resp.GetUsers() {
		if u == nil || u.GetEmail() == "" {
			continue
		}
		item := RuntimeInboundUser{
			InboundTag: inboundTag,
			Email:      u.GetEmail(),
		}
		if u.GetAccount() != nil {
			decoded, decErr := DecodeAccountMessage(u.GetAccount())
			if decErr != nil {
				return nil, fmt.Errorf("decode runtime user %s in %s failed: %w", u.GetEmail(), inboundTag, decErr)
			}
			item.Protocol = decoded.Protocol
			item.UUID = decoded.UUID
			item.Flow = decoded.Flow
			item.Method = decoded.Method
		}
		users = append(users, item)
	}
	return users, nil
}

// GetSysUptime queries Xray StatsService.GetSysStats and returns process uptime in seconds.
func (c *GRPCClient) GetSysUptime(ctx context.Context) (uint32, error) {
	_, statsCli, err := c.getClients(ctx)
	if err != nil {
		return 0, err
	}

	rpcCtx, cancel := ensureRPCTimeout(ctx)
	defer cancel()

	resp, err := statsCli.GetSysStats(rpcCtx, &proto.SysStatsRequest{})
	if err != nil {
		return 0, fmt.Errorf("xray get sys stats failed: %w", err)
	}
	return resp.GetUptime(), nil
}

