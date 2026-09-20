package domain_test

import (
	"crypto/x509"
	"crypto/x509/pkix"
	"errors"
	"net/http"
	"reflect"
	"strings"
	"testing"
	"time"

	"panel/internal/domain"
)

func TestExtractRealityTargets(t *testing.T) {
	t.Run("nil inbound", func(t *testing.T) {
		dest, port, serverNames, isReality := domain.ExtractRealityTargets(nil)
		if isReality {
			t.Errorf("expected isReality=false, got true")
		}
		if dest != "" {
			t.Errorf("expected empty dest, got %s", dest)
		}
		if port != 0 {
			t.Errorf("expected port=0, got %d", port)
		}
		if serverNames != nil {
			t.Errorf("expected nil serverNames, got %v", serverNames)
		}
	})

	t.Run("non-reality inbound", func(t *testing.T) {
		inbound := &domain.Inbound{
			Tag:            "vmess-ws",
			StreamSettings: `{"network":"ws","security":"tls","tlsSettings":{"serverName":"test.com"}}`,
		}
		dest, port, serverNames, isReality := domain.ExtractRealityTargets(inbound)
		if isReality {
			t.Errorf("expected isReality=false, got true")
		}
		if dest != "" || port != 0 || serverNames != nil {
			t.Errorf("expected empty results for non-reality inbound")
		}
	})

	t.Run("reality with dest and serverNames in streamSettings", func(t *testing.T) {
		inbound := &domain.Inbound{
			Tag: "vless-reality",
			StreamSettings: `{
				"network":"tcp",
				"security":"reality",
				"realitySettings":{
					"dest":"gateway.icloud.com:443",
					"serverNames":["gateway.icloud.com","icloud.com"],
					"privateKey":"privkey123"
				}
			}`,
		}
		dest, port, serverNames, isReality := domain.ExtractRealityTargets(inbound)
		if !isReality {
			t.Fatalf("expected isReality=true, got false")
		}
		if dest != "gateway.icloud.com" {
			t.Errorf("expected dest 'gateway.icloud.com', got '%s'", dest)
		}
		if port != 443 {
			t.Errorf("expected port 443, got %d", port)
		}
		expectedSN := []string{"gateway.icloud.com", "icloud.com"}
		if !reflect.DeepEqual(serverNames, expectedSN) {
			t.Errorf("expected serverNames %v, got %v", expectedSN, serverNames)
		}
	})

	t.Run("reality with single serverName and target in settingsJson fallback", func(t *testing.T) {
		inbound := &domain.Inbound{
			Tag:            "vless-fallback",
			StreamSettings: "",
			SettingsJSON: `{
				"security":"reality",
				"realitySettings":{
					"target":"1.1.1.1:8443",
					"serverName":"one.one.one.one"
				}
			}`,
		}
		dest, port, serverNames, isReality := domain.ExtractRealityTargets(inbound)
		if !isReality {
			t.Fatalf("expected isReality=true, got false")
		}
		if dest != "1.1.1.1" {
			t.Errorf("expected dest '1.1.1.1', got '%s'", dest)
		}
		if port != 8443 {
			t.Errorf("expected port 8443, got %d", port)
		}
		expectedSN := []string{"one.one.one.one"}
		if !reflect.DeepEqual(serverNames, expectedSN) {
			t.Errorf("expected serverNames %v, got %v", expectedSN, serverNames)
		}
	})

	t.Run("reality with dest without port defaults to 443", func(t *testing.T) {
		inbound := &domain.Inbound{
			Tag: "vless-default-port",
			StreamSettings: `{
				"security":"reality",
				"realitySettings":{
					"dest":"www.apple.com",
					"serverNames":["www.apple.com"]
				}
			}`,
		}
		dest, port, serverNames, isReality := domain.ExtractRealityTargets(inbound)
		if !isReality {
			t.Fatalf("expected isReality=true, got false")
		}
		if dest != "www.apple.com" {
			t.Errorf("expected dest 'www.apple.com', got '%s'", dest)
		}
		if port != 443 {
			t.Errorf("expected port 443, got %d", port)
		}
		expectedSN := []string{"www.apple.com"}
		if !reflect.DeepEqual(serverNames, expectedSN) {
			t.Errorf("expected serverNames %v, got %v", expectedSN, serverNames)
		}
	})
}

func TestEvaluateRealityProbe(t *testing.T) {
	now := time.Date(2026, 9, 20, 12, 0, 0, 0, time.UTC)

	makeValidCert := func(dnsName string, notAfter time.Time) *x509.Certificate {
		return &x509.Certificate{
			DNSNames: []string{dnsName},
			Subject: pkix.Name{
				CommonName:   dnsName,
				Organization: []string{"Valid Corp"},
			},
			Issuer: pkix.Name{
				CommonName:   "Let's Encrypt Authority",
				Organization: []string{"Let's Encrypt"},
			},
			NotBefore: now.Add(-30 * 24 * time.Hour),
			NotAfter:  notAfter,
		}
	}

	t.Run("TCP unreachable error", func(t *testing.T) {
		tcpErr := errors.New("dial tcp 1.2.3.4:443: i/o timeout")
		status, errType, details := domain.EvaluateRealityProbe(tcpErr, 0x0304, "h2", nil, nil, now, "example.com")
		if status != domain.RealityStatusError {
			t.Errorf("expected status %s, got %s", domain.RealityStatusError, status)
		}
		if errType != domain.ErrTypeTCPUnreachable {
			t.Errorf("expected errType %s, got %s", domain.ErrTypeTCPUnreachable, errType)
		}
		if !strings.Contains(details, "TCP") {
			t.Errorf("expected details to mention TCP, got %s", details)
		}
	})

	t.Run("TLS version lower than 1.3", func(t *testing.T) {
		cert := makeValidCert("example.com", now.Add(30*24*time.Hour))
		// 0x0303 is TLS 1.2
		status, errType, _ := domain.EvaluateRealityProbe(nil, 0x0303, "h2", cert, nil, now, "example.com")
		if status != domain.RealityStatusError {
			t.Errorf("expected status %s, got %s", domain.RealityStatusError, status)
		}
		if errType != domain.ErrTypeTLSVersionLow {
			t.Errorf("expected errType %s, got %s", domain.ErrTypeTLSVersionLow, errType)
		}
	})

	t.Run("Cloudflare CDN detected in cert issuer", func(t *testing.T) {
		cert := &x509.Certificate{
			DNSNames: []string{"example.com"},
			Issuer: pkix.Name{
				Organization: []string{"Cloudflare, Inc."},
				CommonName:   "Cloudflare Inc ECC CA-3",
			},
			NotBefore: now.Add(-30 * 24 * time.Hour),
			NotAfter:  now.Add(30 * 24 * time.Hour),
		}
		status, errType, _ := domain.EvaluateRealityProbe(nil, 0x0304, "h2", cert, nil, now, "example.com")
		if status != domain.RealityStatusError {
			t.Errorf("expected status %s, got %s", domain.RealityStatusError, status)
		}
		if errType != domain.ErrTypeCDNDetected {
			t.Errorf("expected errType %s, got %s", domain.ErrTypeCDNDetected, errType)
		}
	})

	t.Run("Cloudflare CDN detected in HTTP Server header", func(t *testing.T) {
		cert := makeValidCert("example.com", now.Add(30*24*time.Hour))
		headers := http.Header{}
		headers.Set("Server", "cloudflare")
		status, errType, _ := domain.EvaluateRealityProbe(nil, 0x0304, "h2", cert, headers, now, "example.com")
		if status != domain.RealityStatusError {
			t.Errorf("expected status %s, got %s", domain.RealityStatusError, status)
		}
		if errType != domain.ErrTypeCDNDetected {
			t.Errorf("expected errType %s, got %s", domain.ErrTypeCDNDetected, errType)
		}
	})

	t.Run("Cloudflare CDN detected in cf-ray header", func(t *testing.T) {
		cert := makeValidCert("example.com", now.Add(30*24*time.Hour))
		headers := http.Header{}
		headers.Set("cf-ray", "8f1234567890-SJC")
		status, errType, _ := domain.EvaluateRealityProbe(nil, 0x0304, "h2", cert, headers, now, "example.com")
		if status != domain.RealityStatusError {
			t.Errorf("expected status %s, got %s", domain.RealityStatusError, status)
		}
		if errType != domain.ErrTypeCDNDetected {
			t.Errorf("expected errType %s, got %s", domain.ErrTypeCDNDetected, errType)
		}
	})

	t.Run("Certificate domain mismatch", func(t *testing.T) {
		cert := makeValidCert("apple.com", now.Add(30*24*time.Hour))
		status, errType, _ := domain.EvaluateRealityProbe(nil, 0x0304, "h2", cert, nil, now, "microsoft.com")
		if status != domain.RealityStatusError {
			t.Errorf("expected status %s, got %s", domain.RealityStatusError, status)
		}
		if errType != domain.ErrTypeCertDomainMismatch {
			t.Errorf("expected errType %s, got %s", domain.ErrTypeCertDomainMismatch, errType)
		}
	})

	t.Run("Certificate expired", func(t *testing.T) {
		cert := makeValidCert("example.com", now.Add(-1*24*time.Hour))
		status, errType, _ := domain.EvaluateRealityProbe(nil, 0x0304, "h2", cert, nil, now, "example.com")
		if status != domain.RealityStatusError {
			t.Errorf("expected status %s, got %s", domain.RealityStatusError, status)
		}
		if errType != domain.ErrTypeCertExpired {
			t.Errorf("expected errType %s, got %s", domain.ErrTypeCertExpired, errType)
		}
	})

	t.Run("Certificate expiring soon (< 7 days)", func(t *testing.T) {
		cert := makeValidCert("example.com", now.Add(3*24*time.Hour))
		status, errType, _ := domain.EvaluateRealityProbe(nil, 0x0304, "h2", cert, nil, now, "example.com")
		if status != domain.RealityStatusWarning {
			t.Errorf("expected status %s, got %s", domain.RealityStatusWarning, status)
		}
		if errType != domain.ErrTypeCertExpiringSoon {
			t.Errorf("expected errType %s, got %s", domain.ErrTypeCertExpiringSoon, errType)
		}
	})

	t.Run("ALPN missing warning", func(t *testing.T) {
		cert := makeValidCert("example.com", now.Add(30*24*time.Hour))
		status, errType, _ := domain.EvaluateRealityProbe(nil, 0x0304, "", cert, nil, now, "example.com")
		if status != domain.RealityStatusWarning {
			t.Errorf("expected status %s, got %s", domain.RealityStatusWarning, status)
		}
		if errType != domain.ErrTypeALPNMissing {
			t.Errorf("expected errType %s, got %s", domain.ErrTypeALPNMissing, errType)
		}
	})

	t.Run("All compliant ok", func(t *testing.T) {
		cert := makeValidCert("example.com", now.Add(60*24*time.Hour))
		headers := http.Header{}
		headers.Set("Server", "nginx")
		status, errType, details := domain.EvaluateRealityProbe(nil, 0x0304, "h2", cert, headers, now, "example.com")
		if status != domain.RealityStatusOk {
			t.Errorf("expected status %s, got %s", domain.RealityStatusOk, status)
		}
		if errType != "" {
			t.Errorf("expected empty errType, got %s", errType)
		}
		if !strings.Contains(details, "正常") {
			t.Errorf("expected details to contain '正常', got %s", details)
		}
	})
}

func TestBuildRealityCheckItem(t *testing.T) {
	now := time.Date(2026, 9, 20, 12, 0, 0, 0, time.UTC)
	cert := &x509.Certificate{
		NotAfter: now.Add(10 * 24 * time.Hour),
	}

	item := domain.BuildRealityCheckItem(
		1,
		"vless-reality",
		"apple.com",
		443,
		"apple.com",
		domain.RealityStatusOk,
		"",
		"域名检测正常",
		0x0304,
		"h2",
		cert,
		false,
		45,
		now,
	)

	if item.InboundID != 1 {
		t.Errorf("expected InboundID=1, got %d", item.InboundID)
	}
	if item.Dest != "apple.com:443" {
		t.Errorf("expected Dest='apple.com:443', got '%s'", item.Dest)
	}
	if item.TLSVersion != "TLS 1.3" {
		t.Errorf("expected TLSVersion='TLS 1.3', got '%s'", item.TLSVersion)
	}
	if item.DaysLeft != 10 || item.CertDaysLeft != 10 {
		t.Errorf("expected 10 days left, got %d", item.DaysLeft)
	}
}

