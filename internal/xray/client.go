package xray

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	proxymanCommand "github.com/xtls/xray-core/app/proxyman/command"
	statsCommand "github.com/xtls/xray-core/app/stats/command"
	"go.etcd.io/bbolt"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/connectivity"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
)

var (
	ErrClientClosed        = errors.New("xray client is closed")
	ErrStorageNotAvailable = errors.New("bbolt storage is not configured")
	ErrInvalidParameter    = errors.New("invalid parameter")
)

// XrayClient encapsulates gRPC communication with Xray-core's HandlerService and StatsService,
// providing connection state management, epoch-tracked reconnection storm prevention, health checking,
// and ACID storage coordination.
type XrayClient struct {
	addr        string
	mu          sync.RWMutex
	conn        *grpc.ClientConn
	connEpoch   uint64 // Incremented on each reconnect to prevent thundering-herd reconnect storms
	handlerCli  proxymanCommand.HandlerServiceClient
	statsCli    statsCommand.StatsServiceClient
	db          *bbolt.DB
	ownsDB      bool
	initErr     error
	dialTimeout time.Duration
	closed      atomic.Bool

	// Background health checking
	healthMu       sync.Mutex
	healthCancel   context.CancelFunc
	healthDone     chan struct{}
	healthInterval time.Duration
}

// Option configures XrayClient.
type Option func(*XrayClient)

// WithStorage sets an existing BoltDB database handle for ACID local state persistence.
func WithStorage(db *bbolt.DB) Option {
	return func(c *XrayClient) {
		c.db = db
		c.ownsDB = false
	}
}

// WithDBPath opens a new BoltDB file at the specified path for local state persistence.
func WithDBPath(path string) Option {
	return func(c *XrayClient) {
		trimmed := strings.TrimSpace(path)
		if trimmed == "" {
			c.initErr = fmt.Errorf("%w: db path cannot be empty", ErrInvalidParameter)
			return
		}
		if dir := filepath.Dir(trimmed); dir != "" && dir != "." {
			if err := os.MkdirAll(dir, 0755); err != nil {
				c.initErr = fmt.Errorf("failed to create db directory %s: %w", dir, err)
				return
			}
		}
		db, err := bbolt.Open(trimmed, 0600, &bbolt.Options{Timeout: 3 * time.Second})
		if err != nil {
			c.initErr = fmt.Errorf("failed to open bbolt db at %s: %w", trimmed, err)
			return
		}
		c.db = db
		c.ownsDB = true
	}
}

// WithDialTimeout sets custom timeout for gRPC dialing.
func WithDialTimeout(d time.Duration) Option {
	return func(c *XrayClient) {
		if d > 0 {
			c.dialTimeout = d
		}
	}
}

// WithAutoHealthCheck enables periodic background health checks and automatic reconnection.
func WithAutoHealthCheck(interval time.Duration) Option {
	return func(c *XrayClient) {
		if interval > 0 {
			c.healthInterval = interval
		}
	}
}

// NewXrayClient creates a new XrayClient pointing to the specified gRPC endpoint (e.g. "127.0.0.1:10085").
func NewXrayClient(addr string, opts ...Option) (*XrayClient, error) {
	if addr == "" {
		addr = "127.0.0.1:10085"
	}
	client := &XrayClient{
		addr:        addr,
		dialTimeout: 3 * time.Second,
	}

	for _, opt := range opts {
		opt(client)
	}

	if client.initErr != nil {
		return nil, client.initErr
	}

	if client.db != nil {
		if err := client.initStorage(); err != nil {
			if client.ownsDB {
				_ = client.db.Close()
			}
			return nil, fmt.Errorf("failed to initialize bbolt buckets: %w", err)
		}
	}

	if client.healthInterval > 0 {
		client.startBackgroundHealthCheck(client.healthInterval)
	}

	return client, nil
}

// getConn returns a healthy gRPC client connection, dialing or reconnecting if necessary.
func (c *XrayClient) getConn(ctx context.Context) (*grpc.ClientConn, error) {
	if c.closed.Load() {
		return nil, ErrClientClosed
	}

	c.mu.RLock()
	if c.conn != nil {
		state := c.conn.GetState()
		if state != connectivity.Shutdown && state != connectivity.TransientFailure {
			conn := c.conn
			c.mu.RUnlock()
			return conn, nil
		}
	}
	c.mu.RUnlock()

	c.mu.Lock()
	defer c.mu.Unlock()

	if c.closed.Load() {
		return nil, ErrClientClosed
	}

	// Double-check with write lock held
	if c.conn != nil {
		state := c.conn.GetState()
		if state != connectivity.Shutdown && state != connectivity.TransientFailure {
			return c.conn, nil
		}
	}

	return c.dialLocked(ctx)
}

// dialLocked establishes a new connection while caller holds c.mu Write Lock.
func (c *XrayClient) dialLocked(ctx context.Context) (*grpc.ClientConn, error) {
	if c.conn != nil {
		_ = c.conn.Close()
		c.conn = nil
		c.handlerCli = nil
		c.statsCli = nil
	}

	timeout := c.dialTimeout
	if timeout <= 0 {
		timeout = 3 * time.Second
	}
	dialCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	conn, err := grpc.DialContext(
		dialCtx,
		c.addr,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithBlock(),
	)
	if err != nil {
		return nil, fmt.Errorf("connect to xray gRPC dokodemo-door at %s failed: %w", c.addr, err)
	}

	c.conn = conn
	c.connEpoch++
	c.handlerCli = proxymanCommand.NewHandlerServiceClient(conn)
	c.statsCli = statsCommand.NewStatsServiceClient(conn)
	return c.conn, nil
}

// getEpoch returns the current connection epoch safely under read lock.
func (c *XrayClient) getEpoch() uint64 {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.connEpoch
}

// Reconnect terminates any existing gRPC connection and establishes a fresh connection to Xray.
func (c *XrayClient) Reconnect(ctx context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.closed.Load() {
		return ErrClientClosed
	}

	_, err := c.dialLocked(ctx)
	return err
}

// ReconnectAtEpoch reconnects only if the connection has not already been refreshed past seenEpoch.
// This prevents reconnection storms where 20 concurrent failing requests tear down each other's fresh connections.
func (c *XrayClient) ReconnectAtEpoch(ctx context.Context, seenEpoch uint64) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.closed.Load() {
		return ErrClientClosed
	}

	// Another goroutine already reconnected!
	if c.connEpoch > seenEpoch && c.conn != nil {
		state := c.conn.GetState()
		if state != connectivity.Shutdown && state != connectivity.TransientFailure {
			return nil
		}
	}

	_, err := c.dialLocked(ctx)
	return err
}

// CheckHealth performs an active round-trip ping against Xray StatsService to ensure connectivity and responsiveness.
// If the connection has dropped or becomes unresponsive, it attempts automatic recovery.
func (c *XrayClient) CheckHealth(ctx context.Context) error {
	if c.closed.Load() {
		return ErrClientClosed
	}

	conn, err := c.getConn(ctx)
	if err != nil {
		return err
	}

	state := conn.GetState()
	if state == connectivity.Shutdown || state == connectivity.TransientFailure {
		if recErr := c.Reconnect(ctx); recErr != nil {
			return fmt.Errorf("health check failed: connection state %s and reconnect failed: %w", state, recErr)
		}
	}

	c.mu.RLock()
	statsCli := c.statsCli
	c.mu.RUnlock()

	if statsCli == nil {
		return errors.New("stats client not initialized")
	}

	pingCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()

	// Use GetSysStats as the primary lightweight heartbeat ping
	_, err = statsCli.GetSysStats(pingCtx, &statsCommand.SysStatsRequest{})
	if err != nil {
		// Fallback to QueryStats ping with empty pattern
		_, qErr := statsCli.QueryStats(pingCtx, &statsCommand.QueryStatsRequest{Pattern: ""})
		if qErr != nil {
			_ = c.Reconnect(ctx)
			return fmt.Errorf("xray health ping failed: %w", err)
		}
	}

	return nil
}

// Ping is a convenience alias for CheckHealth.
func (c *XrayClient) Ping(ctx context.Context) error {
	return c.CheckHealth(ctx)
}

// IsHealthy returns true if the connection to Xray is currently active and healthy.
func (c *XrayClient) IsHealthy(ctx context.Context) bool {
	return c.CheckHealth(ctx) == nil
}

// isConnectionError checks if an error indicates transport/network breakage.
func isConnectionError(err error) bool {
	if err == nil {
		return false
	}
	s, ok := status.FromError(err)
	if ok {
		switch s.Code() {
		case codes.Unavailable:
			return true
		}
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "broken pipe") ||
		strings.Contains(msg, "connection reset") ||
		strings.Contains(msg, "connection refused") ||
		strings.Contains(msg, "closing transport") ||
		strings.Contains(msg, "transport is closing") ||
		strings.Contains(msg, "use of closed network connection") ||
		strings.Contains(msg, "eof")
}

// withRetry executes an RPC operation, retrying once on connection drop after epoch-aware reconnect.
func (c *XrayClient) withRetry(ctx context.Context, op func() error) error {
	epoch := c.getEpoch()
	err := op()
	if err == nil {
		return nil
	}

	if isConnectionError(err) && !c.closed.Load() {
		if recErr := c.ReconnectAtEpoch(ctx, epoch); recErr == nil {
			return op()
		}
	}
	return err
}

func (c *XrayClient) startBackgroundHealthCheck(interval time.Duration) {
	c.healthMu.Lock()
	defer c.healthMu.Unlock()

	if c.healthCancel != nil {
		c.healthCancel()
		if c.healthDone != nil {
			<-c.healthDone
		}
	}

	ctx, cancel := context.WithCancel(context.Background())
	c.healthCancel = cancel
	c.healthDone = make(chan struct{})

	go func() {
		defer close(c.healthDone)
		ticker := time.NewTicker(interval)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if !c.closed.Load() {
					_ = c.CheckHealth(ctx)
				}
			}
		}
	}()
}

// Close gracefully closes the gRPC connection and any internally managed storage resources.
func (c *XrayClient) Close() error {
	if !c.closed.CompareAndSwap(false, true) {
		return nil
	}

	c.healthMu.Lock()
	if c.healthCancel != nil {
		c.healthCancel()
		if c.healthDone != nil {
			<-c.healthDone
		}
		c.healthCancel = nil
		c.healthDone = nil
	}
	c.healthMu.Unlock()

	c.mu.Lock()
	defer c.mu.Unlock()

	var errs []error
	if c.conn != nil {
		if err := c.conn.Close(); err != nil {
			errs = append(errs, err)
		}
		c.conn = nil
		c.handlerCli = nil
		c.statsCli = nil
	}

	if c.ownsDB && c.db != nil {
		if err := c.db.Close(); err != nil {
			errs = append(errs, err)
		}
		c.db = nil
	}

	if len(errs) > 0 {
		return fmt.Errorf("errors closing xray client: %v", errs)
	}
	return nil
}
