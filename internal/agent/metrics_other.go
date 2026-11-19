//go:build !linux

package agent

import "context"

// Collect degrades safely to zero metrics on non-Linux hosts.
func (m *MetricsCollector) Collect(_ context.Context) HostMetrics {
	return HostMetrics{}
}
