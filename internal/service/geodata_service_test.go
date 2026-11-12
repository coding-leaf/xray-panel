package service

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"
)

type mockCoreController struct {
	version string
	err     error
}

func (m *mockCoreController) GetVersion(ctx context.Context) (string, error) {
	return m.version, m.err
}

func (m *mockCoreController) RestartService(ctx context.Context) error {
	return m.err
}

func TestGeoDataService_GetStatus(t *testing.T) {
	tmpDir := t.TempDir()
	mockCtrl := &mockCoreController{version: "Xray 1.8.24"}

	svc := NewGeoDataService(tmpDir+"/xray", mockCtrl, nil)

	status, err := svc.GetStatus(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if status.CoreVersion != "Xray 1.8.24" {
		t.Errorf("expected core version 'Xray 1.8.24', got '%s'", status.CoreVersion)
	}

	prog := svc.GetProgress()
	if prog.IsUpdating {
		t.Errorf("expected IsUpdating to be false initially")
	}
}

func TestGeoDataService_CustomGeoDataDirFromSettings(t *testing.T) {
	tmpDir := t.TempDir()
	customDir := tmpDir + "/custom_geodata"
	mockCtrl := &mockCoreController{version: "Xray 1.8.24"}
	repo := &mockSettingRepo{
		data: map[string]string{
			"xray_geodata_dir": customDir,
		},
	}

	svc := NewGeoDataService(tmpDir+"/xray", mockCtrl, repo)

	status, err := svc.GetStatus(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if status.TargetDirectory != customDir {
		t.Errorf("expected TargetDirectory to be %q, got %q", customDir, status.TargetDirectory)
	}
}

func TestGeoDataService_NewHTTPClientTimeouts(t *testing.T) {
	client := newGeoDataHTTPClient()
	if client.Timeout < 120*time.Second {
		t.Fatalf("expected generous total client timeout >= 120s, got %v", client.Timeout)
	}
	tr, ok := client.Transport.(*http.Transport)
	if !ok || tr == nil {
		t.Fatalf("expected *http.Transport, got %T", client.Transport)
	}
	if tr.TLSHandshakeTimeout != 15*time.Second {
		t.Fatalf("expected TLSHandshakeTimeout=15s, got %v", tr.TLSHandshakeTimeout)
	}
	if tr.ResponseHeaderTimeout != 30*time.Second {
		t.Fatalf("expected ResponseHeaderTimeout=30s, got %v", tr.ResponseHeaderTimeout)
	}
	if tr.DialContext == nil {
		t.Fatal("expected DialContext to be configured")
	}
}

func TestGeoDataService_DownloadWithProgress_SuccessAndErrorCleanup(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/fail" {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("geodata-binary-payload"))
	}))
	defer srv.Close()

	tmpDir := t.TempDir()
	svc := NewGeoDataService(filepath.Join(tmpDir, "xray"), nil, nil)
	client := srv.Client()

	destOK := filepath.Join(tmpDir, "geoip.dat")
	if err := svc.downloadWithProgress(client, srv.URL+"/ok", destOK, 10, 48, "geoip.dat"); err != nil {
		t.Fatalf("downloadWithProgress failed: %v", err)
	}
	gotBytes, err := os.ReadFile(destOK)
	if err != nil {
		t.Fatalf("read downloaded file failed: %v", err)
	}
	if string(gotBytes) != "geodata-binary-payload" {
		t.Fatalf("unexpected downloaded content: %q", string(gotBytes))
	}
	if _, err := os.Stat(destOK + ".tmp"); !os.IsNotExist(err) {
		t.Fatalf("expected temp file to be renamed away, stat err=%v", err)
	}

	destFail := filepath.Join(tmpDir, "geosite.dat")
	if err := svc.downloadWithProgress(client, srv.URL+"/fail", destFail, 50, 95, "geosite.dat"); err == nil {
		t.Fatal("expected error for HTTP 500 response")
	}
	if _, err := os.Stat(destFail); !os.IsNotExist(err) {
		t.Fatalf("expected failed download not to create destination file, stat err=%v", err)
	}
}
