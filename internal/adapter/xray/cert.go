package xray

import (
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"os"
	"strings"
	"time"
)

type CertInfo struct {
	DomainName string    `json:"domainName"`
	Issuer     string    `json:"issuer"`
	NotBefore  time.Time `json:"notBefore"`
	NotAfter   time.Time `json:"notAfter"`
	DaysLeft   int       `json:"daysLeft"`
	Path       string    `json:"path"`
}

// ParseCertFile 解析 PEM 格式的 X.509 证书文件
func ParseCertFile(certPath string) (*CertInfo, error) {
	data, err := os.ReadFile(certPath)
	if err != nil {
		return nil, fmt.Errorf("read cert file failed: %w", err)
	}

	block, _ := pem.Decode(data)
	if block == nil {
		return nil, fmt.Errorf("failed to decode PEM block from %s", certPath)
	}

	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("parse x509 cert failed: %w", err)
	}

	domain := cert.Subject.CommonName
	if len(cert.DNSNames) > 0 {
		domain = strings.Join(cert.DNSNames, ", ")
	}

	daysLeft := int(time.Until(cert.NotAfter).Hours() / 24)

	return &CertInfo{
		DomainName: domain,
		Issuer:     cert.Issuer.CommonName,
		NotBefore:  cert.NotBefore,
		NotAfter:   cert.NotAfter,
		DaysLeft:   daysLeft,
		Path:       certPath,
	}, nil
}
