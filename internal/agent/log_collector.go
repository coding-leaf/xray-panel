package agent

import (
	"context"
	"fmt"
	"strconv"
	"strings"
)

const (
	defaultLogLines = 100
	maxLogLines     = 1000
)

// LogCollector collects recent log lines for an allowed systemd service.
type LogCollector interface {
	Collect(ctx context.Context, service string, lines int) ([]string, error)
}

// JournalctlLogCollector reads systemd journal logs for whitelisted services via journalctl.
type JournalctlLogCollector struct {
	runner CommandRunner
}

// NewJournalctlLogCollector constructs a JournalctlLogCollector using the provided CommandRunner.
func NewJournalctlLogCollector(runner CommandRunner) *JournalctlLogCollector {
	if runner == nil {
		runner = OSCommandRunner{}
	}
	return &JournalctlLogCollector{runner: runner}
}

// Collect validates the service against a strict whitelist, clamps lines to [1, 1000],
// and executes `journalctl -u <service> -n <lines> --no-pager`.
func (c *JournalctlLogCollector) Collect(ctx context.Context, service string, lines int) ([]string, error) {
	if service != "xray" && service != "xray-agent" {
		return nil, fmt.Errorf("unsupported log service %q: only \"xray\" and \"xray-agent\" are allowed", service)
	}

	if lines <= 0 {
		lines = defaultLogLines
	} else if lines > maxLogLines {
		lines = maxLogLines
	}

	out, err := c.runner.Run(ctx, "journalctl", "-u", service, "-n", strconv.Itoa(lines), "--no-pager")
	if err != nil {
		detail := strings.TrimSpace(string(out))
		if detail != "" {
			return nil, fmt.Errorf("journalctl -u %s failed: %w: %s", service, err, detail)
		}
		return nil, fmt.Errorf("journalctl -u %s failed: %w", service, err)
	}

	normalized := strings.ReplaceAll(string(out), "\r\n", "\n")
	trimmed := strings.TrimRight(normalized, "\n")
	if trimmed == "" {
		return []string{}, nil
	}
	return strings.Split(trimmed, "\n"), nil
}
