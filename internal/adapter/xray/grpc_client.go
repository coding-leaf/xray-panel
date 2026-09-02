package xray

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"panel/internal/domain"

	proxymanCommand "github.com/xtls/xray-core/app/proxyman/command"
	statsCommand "github.com/xtls/xray-core/app/stats/command"
	"github.com/xtls/xray-core/common/protocol"
	"github.com/xtls/xray-core/common/serial"
	"github.com/xtls/xray-core/proxy/shadowsocks"
	"github.com/xtls/xray-core/proxy/trojan"
	"github.com/xtls/xray-core/proxy/vless"
	"github.com/xtls/xray-core/proxy/vmess"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

type GRPCClient struct {
	addr       string
	mu         sync.RWMutex
	conn       *grpc.ClientConn
	handlerCli proxymanCommand.HandlerServiceClient
	statsCli   statsCommand.StatsServiceClient
}

func NewGRPCClient(addr string) *GRPCClient {
	if addr == "" {
		addr = "127.0.0.1:8080"
	}
	return &GRPCClient{
		addr: addr,
	}
}

func (c *GRPCClient) getConn(ctx context.Context) (*grpc.ClientConn, error) {
	c.mu.RLock()
	if c.conn != nil {
		defer c.mu.RUnlock()
		return c.conn, nil
	}
	c.mu.RUnlock()

	c.mu.Lock()
	defer c.mu.Unlock()

	if c.conn != nil {
		return c.conn, nil
	}

	dialCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()

	conn, err := grpc.DialContext(
		dialCtx,
		c.addr,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithBlock(),
	)
	if err != nil {
		return nil, fmt.Errorf("%w: connect gRPC at %s failed: %v", domain.ErrXrayUnavailable, c.addr, err)
	}

	c.conn = conn
	c.handlerCli = proxymanCommand.NewHandlerServiceClient(conn)
	c.statsCli = statsCommand.NewStatsServiceClient(conn)
	return c.conn, nil
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
	_, err := c.getConn(ctx)
	if err != nil {
		return err
	}

	accountMsg, err := buildAccountMessage(inbound, user)
	if err != nil {
		return err
	}

	protoUser := &protocol.User{
		Level:   0,
		Email:   user.Email,
		Account: accountMsg,
	}

	req := &proxymanCommand.AlterInboundRequest{
		Tag: inbound.Tag,
		Operation: serial.ToTypedMessage(&proxymanCommand.AddUserOperation{
			User: protoUser,
		}),
	}

	_, err = c.handlerCli.AlterInbound(ctx, req)
	if err != nil {
		return fmt.Errorf("xray alter inbound add user failed: %w", err)
	}
	return nil
}

// RemoveUser 从指定 Inbound 移除用户
func (c *GRPCClient) RemoveUser(ctx context.Context, inboundTag string, email string) error {
	_, err := c.getConn(ctx)
	if err != nil {
		return err
	}

	req := &proxymanCommand.AlterInboundRequest{
		Tag: inboundTag,
		Operation: serial.ToTypedMessage(&proxymanCommand.RemoveUserOperation{
			Email: email,
		}),
	}

	_, err = c.handlerCli.AlterInbound(ctx, req)
	if err != nil {
		return fmt.Errorf("xray alter inbound remove user failed: %w", err)
	}
	return nil
}

// QueryTrafficStats 查询流量统计
func (c *GRPCClient) QueryTrafficStats(ctx context.Context, reset bool) ([]domain.TrafficStat, error) {
	_, err := c.getConn(ctx)
	if err != nil {
		return nil, err
	}

	req := &statsCommand.QueryStatsRequest{
		Reset_: reset,
	}

	resp, err := c.statsCli.QueryStats(ctx, req)
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

func buildAccountMessage(inbound *domain.Inbound, u *domain.User) (*serial.TypedMessage, error) {
	return BuildAccountMessage(inbound, u)
}

func BuildAccountMessage(inbound *domain.Inbound, u *domain.User) (*serial.TypedMessage, error) {
	protocolName := inbound.Protocol
	switch strings.ToLower(protocolName) {
	case "vless":
		flow := ""
		var streamMap map[string]interface{}
		_ = json.Unmarshal([]byte(inbound.StreamSettings), &streamMap)
		net, _ := streamMap["network"].(string)
		sec, _ := streamMap["security"].(string)
		if (net == "" || net == "tcp") && (sec == "reality" || sec == "tls") {
			flow = "xtls-rprx-vision"
		}

		return serial.ToTypedMessage(&vless.Account{
			Id:   u.UUID,
			Flow: flow,
		}), nil
	case "vmess":
		return serial.ToTypedMessage(&vmess.Account{
			Id: u.UUID,
			SecuritySettings: &protocol.SecurityConfig{
				Type: protocol.SecurityType_AUTO,
			},
		}), nil
	case "trojan":
		return serial.ToTypedMessage(&trojan.Account{
			Password: u.UUID,
		}), nil
	case "shadowsocks":
		cipherType := shadowsocks.CipherType_AES_128_GCM
		if inbound.SettingsJSON != "" {
			var settings map[string]interface{}
			if err := json.Unmarshal([]byte(inbound.SettingsJSON), &settings); err == nil {
				var method string
				if m, ok := settings["method"].(string); ok && m != "" {
					method = m
				} else if c, ok := settings["cipher"].(string); ok && c != "" {
					method = c
				}
				switch strings.ToLower(strings.TrimSpace(method)) {
				case "aes-256-gcm":
					cipherType = shadowsocks.CipherType_AES_256_GCM
				case "chacha20-poly1305", "chacha20-ietf-poly1305":
					cipherType = shadowsocks.CipherType_CHACHA20_POLY1305
				case "xchacha20-poly1305", "xchacha20-ietf-poly1305":
					cipherType = shadowsocks.CipherType_XCHACHA20_POLY1305
				case "none":
					cipherType = shadowsocks.CipherType_NONE
				case "aes-128-gcm":
					cipherType = shadowsocks.CipherType_AES_128_GCM
				default:
					cipherType = shadowsocks.CipherType_AES_128_GCM
				}
			}
		}
		return serial.ToTypedMessage(&shadowsocks.Account{
			Password:   u.UUID,
			CipherType: cipherType,
		}), nil
	default:
		return nil, fmt.Errorf("%w: unsupported protocol %s", domain.ErrInvalidInput, protocolName)
	}
}
