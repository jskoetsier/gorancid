package collect_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"gorancid/pkg/collect"
	"gorancid/pkg/config"
	"gorancid/pkg/devicetype"
)

func TestFallbackCollectorSuccess(t *testing.T) {
	dir := t.TempDir()

	// Write a fake "rancid" script that exits 0 and writes a config file
	script := filepath.Join(dir, "fakerancid")
	content := "#!/bin/sh\necho 'version 15.1' > \"$2.new\"\nexit 0\n"
	_ = os.WriteFile(script, []byte(content), 0755)

	fc := &collect.FallbackCollector{
		Device:    config.Device{Hostname: "sw-01", Type: "ios", Status: "up"},
		Spec:      devicetype.DeviceSpec{Type: "ios"},
		OutDir:    dir,
		RancidBin: script,
	}

	result, err := fc.Run(context.Background())
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if result.Status != collect.StatusSuccess {
		t.Errorf("Status = %v, want StatusSuccess", result.Status)
	}
}

func TestFallbackCollectorFailure(t *testing.T) {
	dir := t.TempDir()

	script := filepath.Join(dir, "failerancid")
	_ = os.WriteFile(script, []byte("#!/bin/sh\nexit 1\n"), 0755)

	fc := &collect.FallbackCollector{
		Device:    config.Device{Hostname: "sw-01", Type: "ios", Status: "up"},
		Spec:      devicetype.DeviceSpec{Type: "ios"},
		OutDir:    dir,
		RancidBin: script,
	}

	result, err := fc.Run(context.Background())
	if err != nil {
		t.Fatalf("unexpected hard error: %v", err)
	}
	if result.Status != collect.StatusFailed {
		t.Errorf("Status = %v, want StatusFailed", result.Status)
	}
	if result.Error == nil {
		t.Error("expected result.Error to be set on failure")
	}
}