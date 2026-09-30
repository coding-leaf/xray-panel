package xray

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"panel/internal/adapter/xray/proto"
	"panel/internal/domain"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
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
	c.handlerCli = proto.NewHandlerServiceClient(conn)
	c.statsCli = proto.NewStatsServiceClient(conn)
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

	req := &proto.QueryStatsRequest{
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

