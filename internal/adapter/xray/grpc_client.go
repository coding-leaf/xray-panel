package xray

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"panel/internal/domain"
	"panel/internal/pkg/logger"

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
func (c *GRPCClient) AddUser(ctx context.Context, inboundTag string, user *domain.User, protocolName string) error {
	_, err := c.getConn(ctx)
	if err != nil {
		return err
	}

	accountMsg, err := buildAccountMessage(protocolName, user)
	if err != nil {
		return err
	}

	protoUser := &protocol.User{
		Level:   0,
		Email:   user.Email,
		Account: accountMsg,
	}

	req := &proxymanCommand.AlterInboundRequest{
		Tag: inboundTag,
		Operation: serial.ToTypedMessage(&proxymanCommand.AddUserOperation{
			User: protoUser,
		}),
	}

	callCtx, cancel := context.WithTimeout(ctx, 4*time.Second)
	defer cancel()

	_, err = c.handlerCli.AlterInbound(callCtx, req)
	if err != nil {
		logger.FromContext(ctx).Error("Xray AddUser failed",
			slog.String("inbound", inboundTag),
			slog.String("email", user.Email),
			slog.String("error", err.Error()),
		)
		return fmt.Errorf("xray AddUser failed: %w", err)
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

	callCtx, cancel := context.WithTimeout(ctx, 4*time.Second)
	defer cancel()

	_, err = c.handlerCli.AlterInbound(callCtx, req)
	if err != nil {
		logger.FromContext(ctx).Warn("Xray RemoveUser warning",
			slog.String("inbound", inboundTag),
			slog.String("email", email),
			slog.String("error", err.Error()),
		)
		return fmt.Errorf("xray RemoveUser failed: %w", err)
	}
	return nil
}

// QueryTrafficStats 查询增量流量统计
func (c *GRPCClient) QueryTrafficStats(ctx context.Context, reset bool) ([]domain.TrafficStat, error) {
	_, err := c.getConn(ctx)
	if err != nil {
		return nil, err
	}

	callCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	resp, err := c.statsCli.QueryStats(callCtx, &statsCommand.QueryStatsRequest{
		Pattern: "",
		Reset_:  reset,
	})
	if err != nil {
		return nil, fmt.Errorf("xray QueryStats failed: %w", err)
	}

	var results []domain.TrafficStat
	for _, stat := range resp.GetStat() {
		name := stat.GetName()
		value := stat.GetValue()
		if value == 0 {
			continue
		}

		// Pattern format in Xray stats:
		// user>>>email>>>traffic>>>uplink | downlink
		// inbound>>>tag>>>traffic>>>uplink | downlink
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

func buildAccountMessage(protocolName string, u *domain.User) (*serial.TypedMessage, error) {
	switch strings.ToLower(protocolName) {
	case "vless":
		return serial.ToTypedMessage(&vless.Account{
			Id:   u.UUID,
			Flow: u.Flow,
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
		return serial.ToTypedMessage(&shadowsocks.Account{
			Password:   u.UUID,
			CipherType: shadowsocks.CipherType_AES_128_GCM,
		}), nil
	default:
		return nil, fmt.Errorf("%w: unsupported protocol %s", domain.ErrInvalidInput, protocolName)
	}
}
