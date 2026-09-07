package xray

import (
	"context"
	"errors"
	"fmt"
	"strings"

	statsCommand "github.com/xtls/xray-core/app/stats/command"
)

// UserTrafficAggregated aggregates uplink and downlink byte counters for a user.
type UserTrafficAggregated struct {
	Email    string `json:"email"`
	Uplink   int64  `json:"uplink"`
	Downlink int64  `json:"downlink"`
}

// QueryTraffic queries real-time traffic statistics from Xray's StatsService.
// pattern: regex or prefix filter (e.g. "user>>>", "inbound>>>", or specific user email). An empty string matches all.
// reset: if true, resets counters in Xray atomically upon querying.
// Returns a map of metric name (e.g. "user>>>alice@test.com>>>traffic>>>uplink") to byte count.
func (c *XrayClient) QueryTraffic(pattern string, reset bool) (map[string]int64, error) {
	return c.QueryTrafficWithContext(context.Background(), pattern, reset)
}

// QueryTrafficWithContext executes QueryTraffic with caller-provided context.
func (c *XrayClient) QueryTrafficWithContext(ctx context.Context, pattern string, reset bool) (map[string]int64, error) {
	_, err := c.getConn(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get gRPC connection: %w", err)
	}

	req := &statsCommand.QueryStatsRequest{
		Pattern: pattern,
		Reset_:  reset,
	}

	var resp *statsCommand.QueryStatsResponse
	err = c.withRetry(ctx, func() error {
		c.mu.RLock()
		stats := c.statsCli
		c.mu.RUnlock()

		if stats == nil {
			return errors.New("stats client not available")
		}
		var rpcErr error
		resp, rpcErr = stats.QueryStats(ctx, req)
		return rpcErr
	})
	if err != nil {
		return nil, fmt.Errorf("xray query stats failed: %w", err)
	}

	result := make(map[string]int64)
	if resp != nil {
		for _, stat := range resp.GetStat() {
			if stat != nil {
				result[stat.GetName()] = stat.GetValue()
			}
		}
	}

	return result, nil
}

// QueryUserTraffic is a convenience method that queries all user traffic ("user>>>")
// and aggregates counters by user email.
func (c *XrayClient) QueryUserTraffic(reset bool) (map[string]*UserTrafficAggregated, error) {
	raw, err := c.QueryTraffic("user>>>", reset)
	if err != nil {
		return nil, err
	}

	users := make(map[string]*UserTrafficAggregated)
	for name, value := range raw {
		// Expected format: user>>><email>>>>traffic>>>[uplink|downlink]
		parts := strings.Split(name, ">>>")
		if len(parts) < 4 {
			continue
		}

		email := parts[1]
		direction := parts[3]

		u, exists := users[email]
		if !exists {
			u = &UserTrafficAggregated{Email: email}
			users[email] = u
		}

		switch direction {
		case "uplink":
			u.Uplink += value
		case "downlink":
			u.Downlink += value
		}
	}

	return users, nil
}
