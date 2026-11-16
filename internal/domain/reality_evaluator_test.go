package domain_test

import (
	"crypto/tls"
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

	t.Run("reality without dest does not infer from serverNames", func(t *testing.T) {
		inbound := &domain.Inbound{
			Tag:      "vless-no-dest",
			Protocol: "vless",
			StreamSettings: `{
				"security":"reality",
				"realitySettings":{
					"serverNames":["www.apple.com"]
				}
			}`,
		}
		dest, _, serverNames, isReality := domain.ExtractRealityTargets(inbound)
		if !isReality {
			t.Fatalf("expected isReality=true, got false")
		}
		if dest != "" {
			t.Errorf("expected empty dest, got %q", dest)
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
		if status != domain.RealityStatusWarning {
			t.Errorf("expected status %s, got %s", domain.RealityStatusWarning, status)
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
		if status != domain.RealityStatusWarning {
			t.Errorf("expected status %s, got %s", domain.RealityStatusWarning, status)
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
		if status != domain.RealityStatusWarning {
			t.Errorf("expected status %s, got %s", domain.RealityStatusWarning, status)
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

	t.Run("bare IPv6 dest formatted with brackets and port", func(t *testing.T) {
		ipv6Item := domain.BuildRealityCheckItem(
			2,
			"vless-ipv6",
			"2606:4700::1",
			443,
			"example.com",
			domain.RealityStatusOk,
			"",
			"ok",
			0x0304,
			"h2",
			cert,
			false,
			20,
			now,
		)
		if ipv6Item.Dest != "[2606:4700::1]:443" {
			t.Errorf("expected Dest='[2606:4700::1]:443', got '%s'", ipv6Item.Dest)
		}
	})
}

func TestEvaluateRealityProbeInput(t *testing.T) {
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

	t.Run("unsupported CurveID P256 returns KEY_EXCHANGE_UNSUPPORTED error", func(t *testing.T) {
		cert := makeValidCert("example.com", now.Add(30*24*time.Hour))
		status, errType, _ := domain.EvaluateRealityProbeInput(domain.RealityProbeInput{
			TLSVersion: tls.VersionTLS13,
			CurveID:    tls.CurveP256,
			ALPN:       "h2",
			Cert:       cert,
			Dest:       "example.com",
			ServerName: "example.com",
		}, now)
		if status != domain.RealityStatusError || errType != domain.ErrTypeKeyExchangeUnsupported {
			t.Fatalf("expected error (%s), got status=%s errType=%s", domain.ErrTypeKeyExchangeUnsupported, status, errType)
		}
	})

	t.Run("supported CurveID X25519 and X25519MLKEM768 pass", func(t *testing.T) {
		cert := makeValidCert("example.com", now.Add(30*24*time.Hour))
		for _, curve := range []tls.CurveID{tls.X25519, tls.X25519MLKEM768} {
			status, errType, _ := domain.EvaluateRealityProbeInput(domain.RealityProbeInput{
				TLSVersion: tls.VersionTLS13,
				CurveID:    curve,
				ALPN:       "h2",
				Cert:       cert,
				Dest:       "example.com",
				ServerName: "example.com",
			}, now)
			if status != domain.RealityStatusOk || errType != "" {
				t.Fatalf("expected ok for curve %v, got status=%s errType=%s", curve, status, errType)
			}
		}
	})

	t.Run("untrusted certificate returns CERT_UNTRUSTED error", func(t *testing.T) {
		cert := makeValidCert("example.com", now.Add(30*24*time.Hour))
		status, errType, _ := domain.EvaluateRealityProbeInput(domain.RealityProbeInput{
			TLSVersion:    tls.VersionTLS13,
			CurveID:       tls.X25519,
			ALPN:          "h2",
			Cert:          cert,
			CertVerifyErr: errors.New("x509: certificate signed by unknown authority"),
			Dest:          "example.com",
			ServerName:    "example.com",
		}, now)
		if status != domain.RealityStatusError || errType != domain.ErrTypeCertUntrusted {
			t.Fatalf("expected error (%s), got status=%s errType=%s", domain.ErrTypeCertUntrusted, status, errType)
		}
	})

	t.Run("CDN providers return warning CDN_DETECTED", func(t *testing.T) {
		cert := makeValidCert("example.com", now.Add(30*24*time.Hour))
		cases := []struct {
			name    string
			headers http.Header
		}{
			{"Cloudflare", http.Header{"Cf-Ray": []string{"8f123456-SJC"}}},
			{"CloudFront", http.Header{"X-Amz-Cf-Id": []string{"abc123"}}},
			{"Akamai", http.Header{"Server": []string{"AkamaiGHost"}}},
			{"Fastly", http.Header{"X-Served-By": []string{"cache-sjc10023-SJC"}}},
		}
		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				status, errType, _ := domain.EvaluateRealityProbeInput(domain.RealityProbeInput{
					TLSVersion: tls.VersionTLS13,
					CurveID:    tls.X25519,
					ALPN:       "h2",
					Cert:       cert,
					Headers:    tc.headers,
					Dest:       "example.com",
					ServerName: "example.com",
				}, now)
				if status != domain.RealityStatusWarning || errType != domain.ErrTypeCDNDetected {
					t.Fatalf("%s: expected warning (%s), got status=%s errType=%s", tc.name, domain.ErrTypeCDNDetected, status, errType)
				}
			})
		}
	})

	t.Run("apple and icloud domains return warning HIGH_RISK_DOMAIN", func(t *testing.T) {
		for _, domainName := range []string{"www.apple.com", "gateway.icloud.com"} {
			cert := makeValidCert(domainName, now.Add(30*24*time.Hour))
			status, errType, _ := domain.EvaluateRealityProbeInput(domain.RealityProbeInput{
				TLSVersion: tls.VersionTLS13,
				CurveID:    tls.X25519,
				ALPN:       "h2",
				Cert:       cert,
				Dest:       domainName,
				ServerName: domainName,
			}, now)
			if status != domain.RealityStatusWarning || errType != domain.ErrTypeHighRiskDomain {
				t.Fatalf("%s: expected warning (%s), got status=%s errType=%s", domainName, domain.ErrTypeHighRiskDomain, status, errType)
			}
		}
	})

	t.Run("HTTP redirect status codes return warning HTTP_REDIRECT", func(t *testing.T) {
		cert := makeValidCert("example.com", now.Add(30*24*time.Hour))
		for _, code := range []int{301, 302, 303, 307, 308} {
			status, errType, _ := domain.EvaluateRealityProbeInput(domain.RealityProbeInput{
				TLSVersion: tls.VersionTLS13,
				CurveID:    tls.X25519,
				ALPN:       "h2",
				Cert:       cert,
				StatusCode: code,
				Dest:       "example.com",
				ServerName: "example.com",
			}, now)
			if status != domain.RealityStatusWarning || errType != domain.ErrTypeHTTPRedirect {
				t.Fatalf("code %d: expected warning (%s), got status=%s errType=%s", code, domain.ErrTypeHTTPRedirect, status, errType)
			}
		}
	})

	t.Run("ALPN http/1.1 returns warning ALPN_H2_MISSING", func(t *testing.T) {
		cert := makeValidCert("example.com", now.Add(30*24*time.Hour))
		status, errType, _ := domain.EvaluateRealityProbeInput(domain.RealityProbeInput{
			TLSVersion: tls.VersionTLS13,
			CurveID:    tls.X25519,
			ALPN:       "http/1.1",
			Cert:       cert,
			StatusCode: 200,
			Dest:       "example.com",
			ServerName: "example.com",
		}, now)
		if status != domain.RealityStatusWarning || errType != domain.ErrTypeALPNH2Missing {
			t.Fatalf("expected warning (%s), got status=%s errType=%s", domain.ErrTypeALPNH2Missing, status, errType)
		}
	})
}
