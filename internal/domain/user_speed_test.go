package domain_test

import (
	"testing"
	"time"

	"panel/internal/domain"
)

func TestUserTrafficReset_TimeWindowFilter(t *testing.T) {
	email := "reset_test@example.com"

	// Initially not recently reset
	if domain.IsUserRecentlyReset(email, 6000) {
		t.Fatalf("expected user to not be recently reset initially")
	}

	// Set speed and record reset
	domain.SetUserRuntimeSpeed(email, 500, 1000, time.Now().UnixMilli())
	domain.RecordUserTrafficReset(email)

	// Immediately after reset, IsUserRecentlyReset must be true
	if !domain.IsUserRecentlyReset(email, 6000) {
		t.Fatalf("expected user to be marked as recently reset")
	}

	// Runtime speed should be reset to 0
	up, down, _, _ := domain.GetUserRuntimeSpeed(email)
	if up != 0 || down != 0 {
		t.Errorf("expected runtime speed to be 0 after reset, got up=%d, down=%d", up, down)
	}

	// After removing user, reset record should be cleaned up
	domain.RemoveUserRuntimeSpeed(email)
	if domain.IsUserRecentlyReset(email, 6000) {
		t.Fatalf("expected user reset record to be cleaned up on remove")
	}
}
