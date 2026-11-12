package reality

import (
	"bufio"
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"io"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	"golang.org/x/net/http2"
)

// ProbeResult stores the network and TLS probing outcome for a Reality target.
type ProbeResult struct {
	TCPError   error
	TLSVersion uint16
	ALPN       string
	Cert       *x509.Certificate
	Headers    http.Header
	LatencyMs  int64
}

// RealityProber defines the interface for probing a Reality destination.
type RealityProber interface {
	Probe(ctx context.Context, dest string, port int, serverName string) (*ProbeResult, error)
}

// DefaultRealityProber implements RealityProber using Go's standard net and crypto/tls packages.
type DefaultRealityProber struct {
	DialTimeout time.Duration
}

// NewDefaultRealityProber creates a DefaultRealityProber with a standard 3-second timeout.
func NewDefaultRealityProber() *DefaultRealityProber {
	return &DefaultRealityProber{
		DialTimeout: 3 * time.Second,
	}
}

// Probe connects to dest:port, performs a TLS handshake with serverName, and inspects HTTP response headers.
func (p *DefaultRealityProber) Probe(ctx context.Context, dest string, port int, serverName string) (*ProbeResult, error) {
	if ctx == nil {
		ctx = context.Background()
	}

	timeout := p.DialTimeout
	if timeout <= 0 {
		timeout = 3 * time.Second
	}

	// Ensure context has a deadline if none exists
	var cancel context.CancelFunc
	if _, ok := ctx.Deadline(); !ok {
		ctx, cancel = context.WithTimeout(ctx, timeout)
		defer cancel()
	}

	// Parse host and port
	targetHost := strings.TrimSpace(dest)
	targetPort := port
	if h, pStr, err := net.SplitHostPort(targetHost); err == nil {
		targetHost = h
		if parsedPort, err := strconv.Atoi(pStr); err == nil && parsedPort > 0 {
			targetPort = parsedPort
		}
	}
	if targetPort <= 0 {
		targetPort = 443
	}
	addr := net.JoinHostPort(targetHost, strconv.Itoa(targetPort))

	sni := strings.TrimSpace(serverName)
	if sni == "" {
		sni = targetHost
	}

	start := time.Now()
	dialer := &net.Dialer{
		Timeout: timeout,
	}

	// 1. TCP connection
	tcpConn, err := dialer.DialContext(ctx, "tcp", addr)
	if err != nil {
		return &ProbeResult{
			TCPError:  err,
			LatencyMs: time.Since(start).Milliseconds(),
			Headers:   make(http.Header),
		}, nil
	}
	defer tcpConn.Close()

	// 2. TLS Handshake
	tlsConfig := &tls.Config{
		ServerName:         sni,
		InsecureSkipVerify: true,
		NextProtos:         []string{"h2", "http/1.1"},
		MinVersion:         tls.VersionTLS10,
	}
	tlsConn := tls.Client(tcpConn, tlsConfig)
	defer tlsConn.Close()

	if deadline, ok := ctx.Deadline(); ok {
		_ = tlsConn.SetDeadline(deadline)
	} else {
		_ = tlsConn.SetDeadline(time.Now().Add(timeout))
	}

	if err := tlsConn.HandshakeContext(ctx); err != nil {
		return &ProbeResult{
			TCPError:  fmt.Errorf("TLS 握手失败: %w", err),
			LatencyMs: time.Since(start).Milliseconds(),
			Headers:   make(http.Header),
		}, nil
	}

	tlsState := tlsConn.ConnectionState()
	result := &ProbeResult{
		TLSVersion: tlsState.Version,
		ALPN:       tlsState.NegotiatedProtocol,
		LatencyMs:  time.Since(start).Milliseconds(),
		Headers:    make(http.Header),
	}
	if len(tlsState.PeerCertificates) > 0 {
		result.Cert = tlsState.PeerCertificates[0]
	}

	// 3. Lightweight HTTP GET to read response headers for CDN detection
	httpDeadline := time.Now().Add(1 * time.Second)
	if ctxDeadline, ok := ctx.Deadline(); ok && ctxDeadline.Before(httpDeadline) {
		httpDeadline = ctxDeadline
	}
	_ = tlsConn.SetDeadline(httpDeadline)

	if tlsState.NegotiatedProtocol == "h2" {
		if headers := probeHTTP2Headers(ctx, tlsConn, sni, httpDeadline); headers != nil {
			result.Headers = headers
		}
		return result, nil
	}

	if headers := probeHTTP1Headers(ctx, tlsConn, sni); headers != nil {
		result.Headers = headers
	}
	return result, nil
}

func buildProbeURL(sni string) string {
	host := sni
	if strings.Contains(host, ":") && !strings.HasPrefix(host, "[") {
		host = "[" + host + "]"
	}
	return "https://" + host + "/"
}

func probeHTTP2Headers(ctx context.Context, tlsConn *tls.Conn, sni string, deadline time.Time) http.Header {
	reqCtx, cancel := context.WithDeadline(ctx, deadline)
	defer cancel()

	tr := &http2.Transport{}
	cc, err := tr.NewClientConn(tlsConn)
	if err != nil {
		return nil
	}
	defer func() {
		_ = cc.Close()
	}()

	req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, buildProbeURL(sni), nil)
	if err != nil {
		return nil
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36")

	resp, err := cc.RoundTrip(req)
	if err != nil || resp == nil {
		return nil
	}
	defer func() {
		_ = resp.Body.Close()
	}()

	if resp.StatusCode > 0 {
		return resp.Header.Clone()
	}
	return nil
}

func probeHTTP1Headers(ctx context.Context, tlsConn *tls.Conn, sni string) http.Header {
	reqStr := fmt.Sprintf("GET / HTTP/1.1\r\nHost: %s\r\nUser-Agent: Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36\r\nConnection: close\r\n\r\n", sni)
	if _, err := io.WriteString(tlsConn, reqStr); err != nil {
		return nil
	}
	reader := bufio.NewReader(tlsConn)
	dummyReq, err := http.NewRequestWithContext(ctx, http.MethodGet, buildProbeURL(sni), nil)
	if err != nil {
		return nil
	}
	resp, err := http.ReadResponse(reader, dummyReq)
	if err != nil || resp == nil {
		return nil
	}
	defer func() {
		_ = resp.Body.Close()
	}()
	return resp.Header.Clone()
}
