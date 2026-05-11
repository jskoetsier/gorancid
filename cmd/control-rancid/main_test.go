package main

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"gorancid/pkg/config"
	"gorancid/pkg/devicetype"
	"gorancid/pkg/git"
)

func TestPushIfConfigured_NoOp(t *testing.T) {
	dir := t.TempDir()
	if err := git.Init(dir); err != nil {
		t.Fatalf("Init: %v", err)
	}

	// Empty GitRemote — should do nothing and not error.
	if err := pushIfConfigured(dir, "", time.Minute); err != nil {
		t.Errorf("pushIfConfigured with empty remote: %v", err)
	}
}

func TestPushIfConfigured_Pushes(t *testing.T) {
	remote := t.TempDir()
	if err := git.InitBare(remote); err != nil {
		t.Fatalf("InitBare: %v", err)
	}

	local := t.TempDir()
	if err := git.Init(local); err != nil {
		t.Fatalf("Init: %v", err)
	}
	file := filepath.Join(local, "switch1.cfg")
	if err := os.WriteFile(file, []byte("hostname switch1\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := git.Add(local, []string{"switch1.cfg"}); err != nil {
		t.Fatalf("Add: %v", err)
	}
	if err := git.Commit(local, "collect switch1"); err != nil {
		t.Fatalf("Commit: %v", err)
	}

	if err := pushIfConfigured(local, remote, time.Minute); err != nil {
		t.Fatalf("pushIfConfigured: %v", err)
	}

	ts, err := git.LastCommitTime(remote, "")
	if err != nil {
		t.Fatalf("LastCommitTime on remote: %v", err)
	}
	if ts.IsZero() {
		t.Error("expected remote to have a commit after pushIfConfigured")
	}
}

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
