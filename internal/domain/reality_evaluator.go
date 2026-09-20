package domain

import (
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// ExtractRealityTargets safely parses an Inbound's StreamSettings or SettingsJSON
// to extract destination address, destination port, serverNames list, and whether reality is enabled.
func ExtractRealityTargets(inbound *Inbound) (dest string, port int, serverNames []string, isReality bool) {
	if inbound == nil {
		return "", 0, nil, false
	}

	rawSettings := strings.TrimSpace(inbound.StreamSettings)
	if rawSettings == "" {
		rawSettings = strings.TrimSpace(inbound.SettingsJSON)
	}
	if rawSettings == "" {
		return "", 0, nil, false
	}

	var rootMap map[string]interface{}
	if err := json.Unmarshal([]byte(rawSettings), &rootMap); err != nil {
		return "", 0, nil, false
	}

	// Security must be "reality"
	sec, _ := rootMap["security"].(string)
	if !strings.EqualFold(sec, "reality") {
		return "", 0, nil, false
	}
	isReality = true

	// realitySettings may be inside streamSettings or directly in root
	rsMap, _ := rootMap["realitySettings"].(map[string]interface{})
	if rsMap == nil {
		return "", 0, nil, true
	}

	// Extract dest or target
	rawDest, _ := rsMap["dest"].(string)
	if rawDest == "" {
		rawDest, _ = rsMap["target"].(string)
	}
	rawDest = strings.TrimSpace(rawDest)

	// Parse host and port from rawDest
	port = 443
	dest = rawDest
	if rawDest != "" {
		if host, pStr, err := net.SplitHostPort(rawDest); err == nil {
			dest = host
			if p, err := strconv.Atoi(pStr); err == nil && p > 0 {
				port = p
			}
		}
	}

	// Extract serverNames
	var snList []string
	if list, ok := rsMap["serverNames"].([]interface{}); ok {
		for _, item := range list {
			if s, ok := item.(string); ok {
				s = strings.TrimSpace(s)
				if s != "" {
					snList = append(snList, s)
				}
			}
		}
	}
	if singleSN, ok := rsMap["serverName"].(string); ok {
		singleSN = strings.TrimSpace(singleSN)
		if singleSN != "" {
			exists := false
			for _, name := range snList {
				if name == singleSN {
					exists = true
					break
				}
			}
			if !exists {
				snList = append(snList, singleSN)
			}
		}
	}

	if dest == "" && len(snList) > 0 {
		dest = snList[0]
	}

	return dest, port, snList, true
}

// EvaluateRealityProbe evaluates the result of a network probe against Reality compliance rules.
// Rules are evaluated in deterministic priority order:
// 1. TCP connectivity failure -> error (TCP_UNREACHABLE)
// 2. TLS negotiation version < 1.3 -> error (TLS_VERSION_LOW)
// 3. CDN features (Cloudflare cert or HTTP headers) -> error (CDN_DETECTED)
// 4. Certificate hostname mismatch -> error (CERT_DOMAIN_MISMATCH)
//    Certificate expired -> error (CERT_EXPIRED)
//    Certificate expiring soon (< 7 days) -> warning (CERT_EXPIRING_SOON)
// 5. Missing ALPN negotiation -> warning (ALPN_MISSING)
// 6. All compliant -> ok
func EvaluateRealityProbe(
	tcpErr error,
	tlsVer uint16,
	alpn string,
	cert *x509.Certificate,
	headers http.Header,
	now time.Time,
	serverName ...string,
) (status RealityDomainStatus, errorType string, details string) {
	// Rule 1: TCP Connectivity
	if tcpErr != nil {
		return RealityStatusError, ErrTypeTCPUnreachable, fmt.Sprintf("TCP 目标不可达: %v", tcpErr)
	}

	// Rule 2: TLS Version (must be TLS 1.3)
	if tlsVer < tls.VersionTLS13 {
		return RealityStatusError, ErrTypeTLSVersionLow, fmt.Sprintf("TLS 协商版本过低 (%s)，Reality 强制要求 TLS 1.3", FormatTLSVersion(tlsVer))
	}

	// Rule 3: Cloudflare CDN Detection
	isCDN, cdnReason := detectCloudflareCDN(cert, headers)
	if isCDN {
		return RealityStatusError, ErrTypeCDNDetected, fmt.Sprintf("目标站点套用了 Cloudflare 公共 CDN (%s)，存在穿透失败与流量拦截风险", cdnReason)
	}

	// Rule 4: Certificate Validity & Expiration
	if cert == nil {
		return RealityStatusError, ErrTypeCertExpired, "未获取到对端 TLS 证书"
	}

	var sni string
	if len(serverName) > 0 {
		sni = strings.TrimSpace(serverName[0])
	}
	if sni != "" {
		if err := cert.VerifyHostname(sni); err != nil {
			return RealityStatusError, ErrTypeCertDomainMismatch, fmt.Sprintf("证书域名与 SNI 不匹配 (%s): %v", sni, err)
		}
	}

	if now.After(cert.NotAfter) {
		return RealityStatusError, ErrTypeCertExpired, fmt.Sprintf("证书已于 %s 过期", cert.NotAfter.Format("2006-01-02 15:04:05"))
	}

	if cert.NotAfter.Sub(now) < 7*24*time.Hour {
		daysLeft := int(cert.NotAfter.Sub(now).Hours() / 24)
		if daysLeft < 0 {
			daysLeft = 0
		}
		return RealityStatusWarning, ErrTypeCertExpiringSoon, fmt.Sprintf("证书即将到期，剩余 %d 天 (过期时间: %s)", daysLeft, cert.NotAfter.Format("2006-01-02 15:04:05"))
	}

	// Rule 5: ALPN Support
	if alpn == "" {
		return RealityStatusWarning, ErrTypeALPNMissing, "TLS 握手未协商出 ALPN (建议对端支持 h2 或 http/1.1)"
	}

	// Rule 6: All Compliant
	return RealityStatusOk, "", "域名检测正常 (支持 TLS 1.3 与 ALPN, 证书有效)"
}

// detectCloudflareCDN checks whether certificate or response headers indicate Cloudflare.
func detectCloudflareCDN(cert *x509.Certificate, headers http.Header) (bool, string) {
	if cert != nil {
		if strings.Contains(strings.ToLower(cert.Issuer.CommonName), "cloudflare") {
			return true, "证书 Issuer 包含 Cloudflare"
		}
		if strings.Contains(strings.ToLower(cert.Subject.CommonName), "cloudflare") {
			return true, "证书 Subject 包含 Cloudflare"
		}
		for _, org := range cert.Issuer.Organization {
			if strings.Contains(strings.ToLower(org), "cloudflare") {
				return true, "证书 Issuer 组织包含 Cloudflare"
			}
		}
		for _, org := range cert.Subject.Organization {
			if strings.Contains(strings.ToLower(org), "cloudflare") {
				return true, "证书 Subject 组织包含 Cloudflare"
			}
		}
	}

	if headers != nil {
		serverHdr := strings.ToLower(headers.Get("Server"))
		if strings.Contains(serverHdr, "cloudflare") {
			return true, "HTTP 响应头 Server: cloudflare"
		}
		if headers.Get("cf-ray") != "" || headers.Get("Cf-Ray") != "" || headers.Get("CF-RAY") != "" {
			return true, "HTTP 响应头命中 cf-ray"
		}
	}

	return false, ""
}

// FormatTLSVersion returns a friendly string for a TLS version uint16.
func FormatTLSVersion(v uint16) string {
	switch v {
	case tls.VersionTLS13:
		return "TLS 1.3"
	case tls.VersionTLS12:
		return "TLS 1.2"
	case tls.VersionTLS11:
		return "TLS 1.1"
	case tls.VersionTLS10:
		return "TLS 1.0"
	default:
		if v == 0 {
			return ""
		}
		return fmt.Sprintf("0x%04x", v)
	}
}

// BuildRealityCheckItem constructs a RealityCheckItem with consistent calculated fields.
func BuildRealityCheckItem(
	inboundID uint,
	inboundTag string,
	dest string,
	port int,
	serverName string,
	status RealityDomainStatus,
	errorType string,
	details string,
	tlsVer uint16,
	alpn string,
	cert *x509.Certificate,
	isCDN bool,
	latencyMs int64,
	checkedAt time.Time,
) RealityCheckItem {
	fullDest := dest
	if port > 0 && !strings.Contains(dest, ":") {
		fullDest = fmt.Sprintf("%s:%d", dest, port)
	}

	item := RealityCheckItem{
		InboundID:   inboundID,
		InboundTag:  inboundTag,
		Dest:        fullDest,
		ServerName:  serverName,
		Port:        port,
		Status:      status,
		ErrorType:   errorType,
		ErrorMsg:    details,
		Details:     details,
		TLSVersion:  FormatTLSVersion(tlsVer),
		ALPN:        alpn,
		IsCDN:       isCDN,
		LatencyMs:   latencyMs,
		CheckedAt:   checkedAt,
	}

	if cert != nil {
		item.CertNotAfter = cert.NotAfter
		item.CertExpiry = cert.NotAfter.Format("2006-01-02 15:04:05")
		days := int(cert.NotAfter.Sub(checkedAt).Hours() / 24)
		if days < 0 {
			days = 0
		}
		item.CertDaysLeft = days
		item.DaysLeft = days
	}

	return item
}
