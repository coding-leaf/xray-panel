package service

import (
	"context"
	"testing"
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

	svc := NewGeoDataService(tmpDir+"/xray", mockCtrl)

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
