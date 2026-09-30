package xray

import (
	"testing"
)

func TestInboundStreamAccessor_VisionFlowResolution(t *testing.T) {
	tests := []struct {
		name         string
		protocol     string
		streamJSON   string
		settingsJSON string
		wantFlow     string
	}{
		{
			name:       "VLESS TCP REALITY default -> vision",
			protocol:   "vless",
			streamJSON: `{"network":"tcp","security":"reality","realitySettings":{"dest":"example.com:443"}}`,
			wantFlow:   "xtls-rprx-vision",
		},
		{
			name:       "VLESS TCP TLS default -> vision",
			protocol:   "vless",
			streamJSON: `{"network":"tcp","security":"tls"}`,
			wantFlow:   "xtls-rprx-vision",
		},
		{
			name:         "VLESS TCP REALITY explicit none -> empty",
			protocol:     "vless",
			streamJSON:   `{"network":"tcp","security":"reality"}`,
			settingsJSON: `{"flow":"none"}`,
			wantFlow:     "",
		},
		{
			name:         "VLESS TCP REALITY custom flow -> custom",
			protocol:     "vless",
			streamJSON:   `{"network":"tcp","security":"reality"}`,
			settingsJSON: `{"flow":"xtls-rprx-vision-udp443"}`,
			wantFlow:     "xtls-rprx-vision-udp443",
		},
		{
			name:       "VLESS WS TLS -> empty (non-tcp never carries flow)",
			protocol:   "vless",
			streamJSON: `{"network":"ws","security":"tls"}`,
			wantFlow:   "",
		},
		{
			name:       "VLESS XHTTP REALITY -> empty",
			protocol:   "vless",
			streamJSON: `{"network":"xhttp","security":"reality"}`,
			wantFlow:   "",
		},
		{
			name:       "Trojan TCP TLS -> empty (non-vless)",
			protocol:   "trojan",
			streamJSON: `{"network":"tcp","security":"tls"}`,
			wantFlow:   "",
		},
		{
			name:       "VLESS TCP none security -> empty",
			protocol:   "vless",
			streamJSON: `{"network":"tcp","security":"none"}`,
			wantFlow:   "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			accessor := NewInboundStreamAccessor(tt.streamJSON, tt.settingsJSON)
			got := accessor.ResolveVisionFlow(tt.protocol)
			if got != tt.wantFlow {
				t.Errorf("ResolveVisionFlow() = %q, want %q", got, tt.wantFlow)
			}
		})
	}
}

func TestInboundStreamAccessor_ShadowsocksMethod(t *testing.T) {
	tests := []struct {
		name         string
		settingsJSON string
		wantMethod   string
	}{
		{
			name:         "default fallback",
			settingsJSON: `{}`,
			wantMethod:   "aes-128-gcm",
		},
		{
			name:         "explicit method",
			settingsJSON: `{"method":"aes-256-gcm"}`,
			wantMethod:   "aes-256-gcm",
		},
		{
			name:         "legacy cipher alias",
			settingsJSON: `{"cipher":"chacha20-poly1305"}`,
			wantMethod:   "chacha20-poly1305",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			accessor := NewInboundStreamAccessor("", tt.settingsJSON)
			if got := accessor.ResolveShadowsocksMethod(); got != tt.wantMethod {
				t.Errorf("ResolveShadowsocksMethod() = %q, want %q", got, tt.wantMethod)
			}
		})
	}
}

func TestInboundStreamAccessor_RealityAndTransports(t *testing.T) {
	streamJSON := `{
		"network": "ws",
		"security": "reality",
		"realitySettings": {
			"serverNames": ["example.com"],
			"privateKey": "OCiaG7JluOeRDE9IIuqPleHWArqqmnKJ_rKTxtjo7mc",
			"shortIds": ["0123456789abcdef"]
		},
		"wsSettings": {
			"path": "/websocket",
			"headers": {"Host": "ws.example.com"}
		},
		"xhttpSettings": {
			"path": "/xhttp-path",
			"mode": "auto",
			"host": "xhttp.example.com"
		}
	}`

	accessor := NewInboundStreamAccessor(streamJSON, "")

	if accessor.Network() != "ws" {
		t.Errorf("Network() = %s, want ws", accessor.Network())
	}
	if accessor.Security() != "reality" {
		t.Errorf("Security() = %s, want reality", accessor.Security())
	}
	if accessor.GetRealityServerName() != "example.com" {
		t.Errorf("GetRealityServerName() = %s, want example.com", accessor.GetRealityServerName())
	}
	if accessor.GetRealityShortID() != "0123456789abcdef" {
		t.Errorf("GetRealityShortID() = %s, want 0123456789abcdef", accessor.GetRealityShortID())
	}
	if accessor.GetRealityPublicKey() == "" {
		t.Errorf("expected auto-derived public key from private key")
	}
	if accessor.GetWSPath() != "/websocket" {
		t.Errorf("GetWSPath() = %s, want /websocket", accessor.GetWSPath())
	}
	if accessor.GetWSHost() != "ws.example.com" {
		t.Errorf("GetWSHost() = %s, want ws.example.com", accessor.GetWSHost())
	}
	if accessor.GetXHTTPPath() != "/xhttp-path" {
		t.Errorf("GetXHTTPPath() = %s, want /xhttp-path", accessor.GetXHTTPPath())
	}
	if accessor.GetXHTTPMode() != "auto" {
		t.Errorf("GetXHTTPMode() = %s, want auto", accessor.GetXHTTPMode())
	}
	if accessor.GetXHTTPHost() != "xhttp.example.com" {
		t.Errorf("GetXHTTPHost() = %s, want xhttp.example.com", accessor.GetXHTTPHost())
	}
}
