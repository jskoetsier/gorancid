package generic

import (
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
