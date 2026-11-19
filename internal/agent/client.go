package agent

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const (
	defaultHTTPTimeout   = 15 * time.Second
	maxSyncResponseBytes = 16 << 20 // 16 MiB
	maxErrorBodyPreview  = 512
)

// Client communicates with the Master control plane over HTTPS using a Bearer token.
type Client struct {
	baseURL    string
	syncURL    string
	token      string
	httpClient *http.Client
}

// NewClient constructs an agent HTTP client targeting masterURL/api/agent/sync.
func NewClient(masterURL, token string, httpClient *http.Client) (*Client, error) {
	base := strings.TrimRight(strings.TrimSpace(masterURL), "/")
	if base == "" {
		return nil, errors.New("master url is required")
	}
	tok := strings.TrimSpace(token)
	if tok == "" {
		return nil, errors.New("agent token is required")
	}
	if httpClient == nil {
		httpClient = &http.Client{Timeout: defaultHTTPTimeout}
	}
	return &Client{
		baseURL:    base,
		syncURL:    base + "/api/agent/sync",
		token:      tok,
		httpClient: httpClient,
	}, nil
}

// Sync posts a heartbeat and optional traffic batch to Master and decodes the SyncResponse.
func (c *Client) Sync(ctx context.Context, req *SyncRequest) (*SyncResponse, error) {
	if req == nil {
		return nil, errors.New("sync request is nil")
	}

	payload, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("marshal sync request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.syncURL, bytes.NewReader(payload))
	if err != nil {
		return nil, fmt.Errorf("create sync http request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+c.token)

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("post sync request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, maxSyncResponseBytes))
	if err != nil {
		return nil, fmt.Errorf("read sync response body: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		preview := strings.TrimSpace(string(body))
		if len(preview) > maxErrorBodyPreview {
			preview = preview[:maxErrorBodyPreview] + "..."
		}
		return nil, fmt.Errorf("master sync returned status %d: %s", resp.StatusCode, preview)
	}

	var syncResp SyncResponse
	if err := json.Unmarshal(body, &syncResp); err != nil {
		return nil, fmt.Errorf("decode sync response: %w", err)
	}
	if !syncResp.OK {
		return nil, errors.New("master sync response ok=false")
	}
	return &syncResp, nil
}

// ReportCommandResult immediately reports a completed asynchronous command result to Master.
func (c *Client) ReportCommandResult(ctx context.Context, res *CommandResult) error {
	if res == nil || strings.TrimSpace(res.ID) == "" {
		return errors.New("command result and id are required")
	}

	payload, err := json.Marshal(res)
	if err != nil {
		return fmt.Errorf("marshal command result: %w", err)
	}

	endpoint := c.baseURL + "/api/agent/commands/" + url.PathEscape(strings.TrimSpace(res.ID)) + "/result"
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(payload))
	if err != nil {
		return fmt.Errorf("create command result http request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+c.token)

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return fmt.Errorf("post command result: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, maxSyncResponseBytes))
	if err != nil {
		return fmt.Errorf("read command result response body: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		preview := strings.TrimSpace(string(body))
		if len(preview) > maxErrorBodyPreview {
			preview = preview[:maxErrorBodyPreview] + "..."
		}
		return fmt.Errorf("master command result returned status %d: %s", resp.StatusCode, preview)
	}
	return nil
}
