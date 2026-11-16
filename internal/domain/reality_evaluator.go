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

	return dest, port, snList, true
}

// RealityProbeInput holds the full probe observations used to evaluate Reality compliance.
type RealityProbeInput struct {
	TCPError      error
	TLSVersion    uint16
	CurveID       tls.CurveID
	ALPN          string
	Cert          *x509.Certificate
	CertVerifyErr error
	StatusCode    int
	Location      string
	Headers       http.Header
	Dest          string
	ServerName    string
}

// EvaluateRealityProbe evaluates the result of a network probe against Reality compliance rules.
// Maintained as a compatibility wrapper around EvaluateRealityProbeInput.
func EvaluateRealityProbe(
	tcpErr error,
	tlsVer uint16,
	alpn string,
	cert *x509.Certificate,
	headers http.Header,
	now time.Time,
	serverName ...string,
) (status RealityDomainStatus, errorType string, details string) {
	var sni string
	if len(serverName) > 0 {
		sni = strings.TrimSpace(serverName[0])
	}
	return EvaluateRealityProbeInput(RealityProbeInput{
		TCPError:   tcpErr,
		TLSVersion: tlsVer,
		ALPN:       alpn,
		Cert:       cert,
		Headers:    headers,
		ServerName: sni,
	}, now)
}

// EvaluateRealityProbeInput evaluates structured probe observations in deterministic priority order:
// Hard errors take precedence over warnings.
func EvaluateRealityProbeInput(
	in RealityProbeInput,
	now time.Time,
) (status RealityDomainStatus, errorType string, details string) {
	// 1. TCP Connectivity
	if in.TCPError != nil {
		return RealityStatusError, ErrTypeTCPUnreachable, fmt.Sprintf("TCP 目标不可达: %v", in.TCPError)
	}

	// 2. TLS Version (must be TLS 1.3)
	if in.TLSVersion < tls.VersionTLS13 {
		return RealityStatusError, ErrTypeTLSVersionLow, fmt.Sprintf("TLS 协商版本过低 (%s)，Reality 强制要求 TLS 1.3", FormatTLSVersion(in.TLSVersion))
	}

	// 3. Key Exchange Curve (Reality requires X25519 or X25519MLKEM768)
	if in.CurveID != 0 && in.CurveID != tls.X25519 && in.CurveID != tls.X25519MLKEM768 {
		return RealityStatusError, ErrTypeKeyExchangeUnsupported, fmt.Sprintf("密钥交换曲线不兼容 (%v)，Reality 强制要求 X25519 或 X25519MLKEM768", in.CurveID)
	}

	// 4. Certificate Validity, Hostname, Expiration & Trust Chain
	if in.Cert == nil {
		return RealityStatusError, ErrTypeCertExpired, "未获取到对端 TLS 证书"
	}

	sni := strings.TrimSpace(in.ServerName)
	if sni != "" {
		if err := in.Cert.VerifyHostname(sni); err != nil {
			return RealityStatusError, ErrTypeCertDomainMismatch, fmt.Sprintf("证书域名与 SNI 不匹配 (%s): %v", sni, err)
		}
	}

	if now.After(in.Cert.NotAfter) {
		return RealityStatusError, ErrTypeCertExpired, fmt.Sprintf("证书已于 %s 过期", in.Cert.NotAfter.Format("2006-01-02 15:04:05"))
	}

	if in.CertVerifyErr != nil {
		return RealityStatusError, ErrTypeCertUntrusted, fmt.Sprintf("证书链不受系统信任 (可能为自签名或缺少中间证书): %v", in.CertVerifyErr)
	}

	// 5. High-risk big-tech disguise targets (Apple / iCloud)
	if isHighRiskRealityDomain(in.Dest, sni) {
		return RealityStatusWarning, ErrTypeHighRiskDomain, "目标属于高风险大厂域名 (Apple/iCloud)，存在被重点封锁及机房 ASN 割裂风险，建议更换同地区低风险站点"
	}

	// 6. CDN Detection (warning: protocol works, but ASN mismatch & WAF risk)
	if isCDN, cdnReason := detectCDN(in.Cert, in.Headers); isCDN {
		return RealityStatusWarning, ErrTypeCDNDetected, fmt.Sprintf("目标站点使用了公共 CDN (%s)，协议可用但存在 ASN 割裂与 WAF 拦截风险，不推荐作为伪装目标", cdnReason)
	}

	// 7. Certificate Expiring Soon (< 7 days)
	if in.Cert.NotAfter.Sub(now) < 7*24*time.Hour {
		daysLeft := int(in.Cert.NotAfter.Sub(now).Hours() / 24)
		if daysLeft < 0 {
			daysLeft = 0
		}
		return RealityStatusWarning, ErrTypeCertExpiringSoon, fmt.Sprintf("证书即将到期，剩余 %d 天 (过期时间: %s)", daysLeft, in.Cert.NotAfter.Format("2006-01-02 15:04:05"))
	}

	// 8. HTTP Redirect
	if isHTTPRedirectStatus(in.StatusCode) {
		loc := strings.TrimSpace(in.Location)
		if loc == "" && in.Headers != nil {
			loc = strings.TrimSpace(in.Headers.Get("Location"))
		}
		if loc != "" {
			return RealityStatusWarning, ErrTypeHTTPRedirect, fmt.Sprintf("目标站点返回 HTTP %d 重定向 (-> %s)，建议选用直接返回 200 的伪装目标", in.StatusCode, loc)
		}
		return RealityStatusWarning, ErrTypeHTTPRedirect, fmt.Sprintf("目标站点返回 HTTP %d 重定向，建议选用直接返回 200 的伪装目标", in.StatusCode)
	}

	// 9. ALPN Support (missing vs non-h2)
	if in.ALPN == "" {
		return RealityStatusWarning, ErrTypeALPNMissing, "TLS 握手未协商出 ALPN (建议对端支持 h2)"
	}
	if in.ALPN != "h2" {
		return RealityStatusWarning, ErrTypeALPNH2Missing, fmt.Sprintf("TLS 仅协商出 %s 而非 h2，建议选用支持 HTTP/2 (h2) 的伪装目标", in.ALPN)
	}

	// 10. All Compliant
	return RealityStatusOk, "", "域名检测正常 (支持 TLS 1.3 / X25519 / h2, 证书有效)"
}

func isHighRiskRealityDomain(dest, serverName string) bool {
	for _, candidate := range []string{dest, serverName} {
		lower := strings.ToLower(strings.TrimSpace(candidate))
		if lower == "" {
			continue
		}
		if strings.Contains(lower, "apple") || strings.Contains(lower, "icloud") {
			return true
		}
	}
	return false
}

func isHTTPRedirectStatus(code int) bool {
	switch code {
	case http.StatusMovedPermanently,
		http.StatusFound,
		http.StatusSeeOther,
		http.StatusTemporaryRedirect,
		http.StatusPermanentRedirect:
		return true
	default:
		return false
	}
}

// detectCDN checks whether certificate or response headers indicate a public CDN (Cloudflare, CloudFront, Akamai, Fastly).
func detectCDN(cert *x509.Certificate, headers http.Header) (bool, string) {
	if cert != nil {
		if strings.Contains(strings.ToLower(cert.Issuer.CommonName), "cloudflare") {
			return true, "Cloudflare (证书 Issuer)"
		}
		if strings.Contains(strings.ToLower(cert.Subject.CommonName), "cloudflare") {
			return true, "Cloudflare (证书 Subject)"
		}
		for _, org := range cert.Issuer.Organization {
			if strings.Contains(strings.ToLower(org), "cloudflare") {
				return true, "Cloudflare (证书 Issuer 组织)"
			}
		}
		for _, org := range cert.Subject.Organization {
			if strings.Contains(strings.ToLower(org), "cloudflare") {
				return true, "Cloudflare (证书 Subject 组织)"
			}
		}
	}

	if headers != nil {
		serverHdr := strings.ToLower(headers.Get("Server"))
		if strings.Contains(serverHdr, "cloudflare") {
			return true, "Cloudflare (Server: cloudflare)"
		}
		if headers.Get("cf-ray") != "" {
			return true, "Cloudflare (cf-ray)"
		}
		if headers.Get("x-amz-cf-id") != "" || headers.Get("x-amz-cf-pop") != "" || strings.Contains(strings.ToLower(headers.Get("Via")), "cloudfront") {
			return true, "CloudFront"
		}
		if strings.Contains(serverHdr, "akamaighost") || headers.Get("x-akamai-transformed") != "" {
			return true, "Akamai"
		}
		if headers.Get("x-served-by") != "" || headers.Get("x-fastly-request-id") != "" || strings.Contains(strings.ToLower(headers.Get("Via")), "fastly") {
			return true, "Fastly"
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
	fullDest := strings.TrimSpace(dest)
	if port > 0 && fullDest != "" {
		if _, _, err := net.SplitHostPort(fullDest); err != nil {
			host := strings.TrimSuffix(strings.TrimPrefix(fullDest, "["), "]")
			fullDest = net.JoinHostPort(host, strconv.Itoa(port))
		}
	}

	item := RealityCheckItem{
		InboundID:  inboundID,
		InboundTag: inboundTag,
		Dest:       fullDest,
		ServerName: serverName,
		Port:       port,
		Status:     status,
		ErrorType:  errorType,
		ErrorMsg:   details,
		Details:    details,
		TLSVersion: FormatTLSVersion(tlsVer),
		ALPN:       alpn,
		IsCDN:      isCDN,
		LatencyMs:  latencyMs,
		CheckedAt:  checkedAt,
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
