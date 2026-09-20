package service

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"errors"
	"math/big"
	"net/http"
	"sync"
	"testing"
	"time"

	"panel/internal/adapter/reality"
	"panel/internal/domain"
)

type mockRealityProber struct {
	mu      sync.Mutex
	results map[string]*reality.ProbeResult
	errs    map[string]error
	calls   int
}

func (m *mockRealityProber) Probe(ctx context.Context, dest string, port int, serverName string) (*reality.ProbeResult, error) {
	m.mu.Lock()
	m.calls++
	m.mu.Unlock()

	key := serverName
	if res, ok := m.results[key]; ok {
		return res, nil
	}
	if err, ok := m.errs[key]; ok {
		return nil, err
	}
	return nil, errors.New("target unreachable")
}

func makeValidCert(serverName string, notAfter time.Time) *x509.Certificate {
	return &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject: pkix.Name{
			CommonName: serverName,
		},
		DNSNames:  []string{serverName},
		NotBefore: time.Now().Add(-24 * time.Hour),
		NotAfter:  notAfter,
	}
}

func TestRealityMonitorService_CheckAll_NormalAndAbnormal(t *testing.T) {
	ctx := context.Background()
	notifier := &mockNotifier{}
	alertSvc := NewAlertService(notifier, nil, nil, nil)

	inboundRepo := &mockInboundRepo{
		inbounds: []domain.Inbound{
			{
				ID:       1,
				Tag:      "vless-reality-ok",
				Protocol: "vless",
				StreamSettings: `{
					"security": "reality",
					"realitySettings": {
						"dest": "gateway.icloud.com:443",
						"serverNames": ["gateway.icloud.com"]
					}
				}`,
			},
			{
				ID:       2,
				Tag:      "vless-reality-bad",
				Protocol: "vless",
				StreamSettings: `{
					"security": "reality",
					"realitySettings": {
						"dest": "bad.tls12.com:443",
						"serverNames": ["bad.tls12.com"]
					}
				}`,
			},
			{
				ID:       3,
				Tag:      "vmess-tcp-plain",
				Protocol: "vmess",
				StreamSettings: `{
					"security": "none"
				}`,
			},
		},
	}

	prober := &mockRealityProber{
		results: map[string]*reality.ProbeResult{
			"gateway.icloud.com": {
				TLSVersion: tls.VersionTLS13,
				ALPN:       "h2",
				Cert:       makeValidCert("gateway.icloud.com", time.Now().Add(60*24*time.Hour)),
				LatencyMs:  30,
				Headers:    make(http.Header),
			},
			"bad.tls12.com": {
				TLSVersion: tls.VersionTLS12,
				ALPN:       "http/1.1",
				Cert:       makeValidCert("bad.tls12.com", time.Now().Add(60*24*time.Hour)),
				LatencyMs:  50,
				Headers:    make(http.Header),
			},
		},
		errs: make(map[string]error),
	}

	svc := NewRealityMonitorService(inboundRepo, prober, alertSvc)

	summary, err := svc.CheckAll(ctx)
	if err != nil {
		t.Fatalf("unexpected CheckAll error: %v", err)
	}

	if summary.TotalChecked != 2 {
		t.Fatalf("expected totalChecked 2, got %d", summary.TotalChecked)
	}
	if summary.OkCount != 1 {
		t.Fatalf("expected okCount 1, got %d", summary.OkCount)
	}
	if summary.ErrorCount != 1 {
		t.Fatalf("expected errorCount 1, got %d", summary.ErrorCount)
	}
	if summary.WarningCount != 0 {
		t.Fatalf("expected warningCount 0, got %d", summary.WarningCount)
	}

	// 验证 GetStatus 返回快照
	cached := svc.GetStatus()
	if cached.TotalChecked != summary.TotalChecked || cached.OkCount != summary.OkCount {
		t.Fatalf("cached status does not match CheckAll output: %+v vs %+v", cached, summary)
	}

	// 验证告警联动：bad.tls12.com 会触发告警
	if len(notifier.messages) != 1 {
		t.Fatalf("expected 1 alert message triggered, got %d", len(notifier.messages))
	}
}

func TestRealityMonitorService_EmptyInbounds(t *testing.T) {
	ctx := context.Background()
	inboundRepo := &mockInboundRepo{inbounds: []domain.Inbound{}}
	prober := &mockRealityProber{
		results: make(map[string]*reality.ProbeResult),
		errs:    make(map[string]error),
	}
	svc := NewRealityMonitorService(inboundRepo, prober, nil)

	summary, err := svc.CheckAll(ctx)
	if err != nil {
		t.Fatalf("unexpected CheckAll error: %v", err)
	}
	if summary.TotalChecked != 0 {
		t.Fatalf("expected 0 checked items, got %d", summary.TotalChecked)
	}
	if len(summary.Items) != 0 {
		t.Fatalf("expected empty items slice, got %v", summary.Items)
	}
}

func TestRealityMonitorService_RaceAndConcurrentAccess(t *testing.T) {
	ctx := context.Background()
	inboundRepo := &mockInboundRepo{
		inbounds: []domain.Inbound{
			{
				ID:  1,
				Tag: "vless-1",
				StreamSettings: `{
					"security": "reality",
					"realitySettings": {
						"dest": "apple.com:443",
						"serverNames": ["apple.com", "icloud.com"]
					}
				}`,
			},
		},
	}

	prober := &mockRealityProber{
		results: map[string]*reality.ProbeResult{
			"apple.com": {
				TLSVersion: tls.VersionTLS13,
				ALPN:       "h2",
				Cert:       makeValidCert("apple.com", time.Now().Add(30*24*time.Hour)),
				LatencyMs:  20,
				Headers:    make(http.Header),
			},
			"icloud.com": {
				TLSVersion: tls.VersionTLS13,
				ALPN:       "h2",
				Cert:       makeValidCert("icloud.com", time.Now().Add(30*24*time.Hour)),
				LatencyMs:  25,
				Headers:    make(http.Header),
			},
		},
		errs: make(map[string]error),
	}

	svc := NewRealityMonitorService(inboundRepo, prober, nil)

	var wg sync.WaitGroup
	// 启动多个读 goroutine 持续 GetStatus
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 50; j++ {
				status := svc.GetStatus()
				_ = status.TotalChecked
			}
		}()
	}

	// 启动多个并发 CheckAll
	for i := 0; i < 3; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 5; j++ {
				_, _ = svc.CheckAll(ctx)
			}
		}()
	}

	wg.Wait()
}
