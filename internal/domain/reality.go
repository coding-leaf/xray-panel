package domain

import "time"

// RealityDomainStatus represents the health evaluation status of a Reality domain.
type RealityDomainStatus string

const (
	RealityStatusOk      RealityDomainStatus = "ok"
	RealityStatusWarning RealityDomainStatus = "warning"
	RealityStatusError   RealityDomainStatus = "error"
)

// Error type constants for Reality domain evaluation.
const (
	ErrTypeTCPUnreachable     = "TCP_UNREACHABLE"
	ErrTypeTLSVersionLow      = "TLS_VERSION_LOW"
	ErrTypeCDNDetected        = "CDN_DETECTED"
	ErrTypeCertExpired        = "CERT_EXPIRED"
	ErrTypeCertExpiringSoon   = "CERT_EXPIRING_SOON"
	ErrTypeCertDomainMismatch = "CERT_DOMAIN_MISMATCH"
	ErrTypeALPNMissing        = "ALPN_MISSING"

	// RealityErr* aliases for consistency across packages
	RealityErrTCPUnreachable     = ErrTypeTCPUnreachable
	RealityErrTLSVersionLow      = ErrTypeTLSVersionLow
	RealityErrCDNDetected        = ErrTypeCDNDetected
	RealityErrCertExpired        = ErrTypeCertExpired
	RealityErrCertExpiringSoon   = ErrTypeCertExpiringSoon
	RealityErrCertDomainMismatch = ErrTypeCertDomainMismatch
	RealityErrALPNMissing        = ErrTypeALPNMissing
)

// RealityCheckItem records the detailed check result for a single Reality serverName.
type RealityCheckItem struct {
	InboundID    uint                `json:"inboundId,omitempty"`
	InboundTag   string              `json:"inboundTag"`
	Dest         string              `json:"dest"`
	ServerName   string              `json:"serverName"`
	Port         int                 `json:"port"`
	Status       RealityDomainStatus `json:"status"`               // ok | warning | error
	ErrorType    string              `json:"errorType,omitempty"`  // TCP_UNREACHABLE, TLS_VERSION_LOW, ALPN_MISSING, CERT_EXPIRED, CERT_EXPIRING_SOON, CDN_DETECTED, CERT_DOMAIN_MISMATCH
	ErrorMsg     string              `json:"errorMsg,omitempty"`   // Human readable error message
	Details      string              `json:"details,omitempty"`    // Detailed description / reason
	TLSVersion   string              `json:"tlsVersion,omitempty"` // e.g. "TLS 1.3"
	ALPN         string              `json:"alpn,omitempty"`       // e.g. "h2"
	CertNotAfter time.Time           `json:"certNotAfter,omitempty"`
	CertExpiry   string              `json:"certExpiry,omitempty"` // Formatted date e.g. "2026-10-15 12:00:00"
	CertDaysLeft int                 `json:"certDaysLeft"`         // Days remaining for certificate
	DaysLeft     int                 `json:"daysLeft"`             // Alias for CertDaysLeft
	IsCDN        bool                `json:"isCdn"`                // Whether CDN (Cloudflare) was detected
	LatencyMs    int64               `json:"latencyMs"`            // Handshake latency in ms
	CheckedAt    time.Time           `json:"checkedAt"`
}

// RealitySummaryStatus represents the aggregated summary snapshot of all checked Reality domains.
type RealitySummaryStatus struct {
	TotalChecked int                `json:"totalChecked"`
	TotalCount   int                `json:"totalCount"`
	OkCount      int                `json:"okCount"`
	WarningCount int                `json:"warningCount"`
	ErrorCount   int                `json:"errorCount"`
	Items        []RealityCheckItem `json:"items"`
	LastCheckAt  time.Time          `json:"lastCheckAt"`
	CheckedAt    time.Time          `json:"checkedAt"`
}
