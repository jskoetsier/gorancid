package main

import (
	"testing"

	"gorancid/pkg/config"
	"gorancid/pkg/devicetype"
)

func TestSelectDevices(t *testing.T) {
	typeSpecs := map[string]devicetype.DeviceSpec{
		"ios":   {Type: "ios"},
		"junos": {Type: "junos"},
	}

	tests := []struct {
		name       string
		devices    []config.Device
		onlyDevice string
		wantCount  int
		wantSkip   int
	}{
		{
			name: "all up known types",
			devices: []config.Device{
				{Hostname: "r1", Type: "ios", Status: "up"},
				{Hostname: "r2", Type: "junos", Status: "up"},
			},
			wantCount: 2,
			wantSkip:  0,
		},
		{
			name: "down device skipped",
			devices: []config.Device{
				{Hostname: "r1", Type: "ios", Status: "up"},
				{Hostname: "r2", Type: "junos", Status: "down"},
			},
			wantCount: 1,
			wantSkip:  0,
		},
		{
			name: "unknown type skipped",
			devices: []config.Device{
				{Hostname: "r1", Type: "ios", Status: "up"},
				{Hostname: "r2", Type: "unknown", Status: "up"},
			},
			wantCount: 1,
			wantSkip:  1,
		},
		{
			name: "onlyDevice filter matches",
			devices: []config.Device{
				{Hostname: "r1", Type: "ios", Status: "up"},
				{Hostname: "r2", Type: "junos", Status: "up"},
			},
			onlyDevice: "r2",
			wantCount:  1,
			wantSkip:   0,
		},
		{
			name: "onlyDevice filter misses",
			devices: []config.Device{
				{Hostname: "r1", Type: "ios", Status: "up"},
			},
			onlyDevice: "r2",
			wantCount:  0,
			wantSkip:   0,
		},
		{
			name:      "empty devices",
			devices:   []config.Device{},
			wantCount: 0,
			wantSkip:  0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			selected, specs, creds, skipped := selectDevices(tt.devices, typeSpecs, nil, tt.onlyDevice)
			if len(selected) != tt.wantCount {
				t.Errorf("selected count = %d, want %d", len(selected), tt.wantCount)
			}
			if len(specs) != tt.wantCount {
				t.Errorf("specs count = %d, want %d", len(specs), tt.wantCount)
			}
			if len(creds) != tt.wantCount {
				t.Errorf("creds count = %d, want %d", len(creds), tt.wantCount)
			}
			if len(skipped) != tt.wantSkip {
				t.Errorf("skipped count = %d, want %d", len(skipped), tt.wantSkip)
			}
		})
	}
}
