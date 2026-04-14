package generic

import (
	"strings"
	"testing"

	"gorancid/pkg/parse"
)

func TestParseFiltersTerminalNoise(t *testing.T) {
	p := New("riverstone")
	out := []byte("show version\r\n\x1b[2KVersion 1.2.3\r\n--More--\r\nprompt# \r\n")
	cfg, err := p.Parse(out, parse.FilterOpts{})
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if len(cfg.Lines) != 3 {
		t.Fatalf("lines = %d, want 3", len(cfg.Lines))
	}
	if cfg.Metadata["parser"] != "generic" {
		t.Fatalf("parser metadata = %q, want generic", cfg.Metadata["parser"])
	}
}

func TestDeviceOptsPromptPattern(t *testing.T) {
	p := New("riverstone")
	opts := p.DeviceOpts()
	if opts.DeviceType != "riverstone" {
		t.Fatalf("DeviceType = %q", opts.DeviceType)
	}
	if opts.PromptPattern == "" || !strings.Contains(opts.PromptPattern, "#") {
		t.Fatalf("expected non-empty prompt pattern with shell markers, got %q", opts.PromptPattern)
	}
}
