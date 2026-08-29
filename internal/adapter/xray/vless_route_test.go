package xray_test

import (
	"testing"

	"panel/internal/adapter/xray"
)

func TestApplyVlessRouteToUUID(t *testing.T) {
	baseUUID := "7117295b-4362-0000-a133-b969344dfcd5"

	tests := []struct {
		name     string
		rawUUID  string
		routeID  uint16
		expected string
	}{
		{
			name:     "Route ID 0 returns unchanged",
			rawUUID:  baseUUID,
			routeID:  0,
			expected: "7117295b-4362-0000-a133-b969344dfcd5",
		},
		{
			name:     "Route ID 1 replaces 3rd section with 0001",
			rawUUID:  baseUUID,
			routeID:  1,
			expected: "7117295b-4362-0001-a133-b969344dfcd5",
		},
		{
			name:     "Route ID 2 replaces 3rd section with 0002",
			rawUUID:  baseUUID,
			routeID:  2,
			expected: "7117295b-4362-0002-a133-b969344dfcd5",
		},
		{
			name:     "Route ID 256 replaces 3rd section with 0100",
			rawUUID:  baseUUID,
			routeID:  256,
			expected: "7117295b-4362-0100-a133-b969344dfcd5",
		},
		{
			name:     "Route ID 65535 replaces 3rd section with ffff",
			rawUUID:  baseUUID,
			routeID:  65535,
			expected: "7117295b-4362-ffff-a133-b969344dfcd5",
		},
		{
			name:     "Invalid UUID returns raw",
			rawUUID:  "invalid-uuid-string",
			routeID:  1,
			expected: "invalid-uuid-string",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			actual := xray.ApplyVlessRouteToUUID(tt.rawUUID, tt.routeID)
			if actual != tt.expected {
				t.Errorf("ApplyVlessRouteToUUID(%q, %d) = %q; want %q", tt.rawUUID, tt.routeID, actual, tt.expected)
			}
		})
	}
}
