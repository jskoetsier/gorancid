package aeos

import (
	"regexp"
	"strings"
	"testing"

	"gorancid/pkg/connect"
	"gorancid/pkg/parse"
)

// ---------------------------------------------------------------------------
// Registration
// ---------------------------------------------------------------------------

func TestRegistration(t *testing.T) {
	p, ok := parse.Lookup("aeos")
	if !ok {
		t.Fatal("aeos parser not registered")
	}
	if _, ok := p.(*EOSParser); !ok {
		t.Fatalf("expected *EOSParser, got %T", p)
	}
}

// ---------------------------------------------------------------------------
// DeviceOpts
// ---------------------------------------------------------------------------

func TestDeviceOpts(t *testing.T) {
	p := &EOSParser{}
	opts := p.DeviceOpts()

	if opts.DeviceType != "aeos" {
		t.Errorf("DeviceType = %q, want %q", opts.DeviceType, "aeos")
	}
	if opts.EnableCmd != "" {
		t.Errorf("EnableCmd = %q, want empty", opts.EnableCmd)
	}
	if len(opts.SetupCommands) != 2 || opts.SetupCommands[0] != "terminal length 0" || opts.SetupCommands[1] != "terminal width 0" {
		t.Errorf("SetupCommands = %v, want [terminal length 0, terminal width 0]", opts.SetupCommands)
	}
}

func TestDeviceOptsPromptPattern(t *testing.T) {
	p := &EOSParser{}
	opts := p.DeviceOpts()

	tests := []struct {
		prompt  string
		matches bool
	}{
		{"am6-lfs-a06-p01#", true},
		{"[16:23] am6-lfs-a06-p01#", true},
		{"switch(config)#", true},
		{"[09:01] switch(config-if)#", true},
		{"switch>", true},
		{"just some text", false},
		{"no prompt at all", false},
	}

	re := compileRegexp(t, opts.PromptPattern)
	for _, tc := range tests {
		// Prepend newline since pattern expects it
		got := re.MatchString("\n" + tc.prompt)
		if got != tc.matches {
			t.Errorf("prompt %q: got matches=%v, want %v", tc.prompt, got, tc.matches)
		}
	}
}

// ---------------------------------------------------------------------------
// Show Version
// ---------------------------------------------------------------------------

func TestShowVersion(t *testing.T) {
	input := `show version
Arista DCS-7280SR-48C6-R
Hardware version: 10.01
Serial number: JPE17151856
Hardware MAC address: 2899.3a41.c58f
System MAC address: 2899.3a41.c58f

Software image version: 4.31.6M
Architecture: i686
Internal build version: 4.31.6M-39956901.4316M
Internal build ID: cdb8a50a-dbee-4c32-9ae6-33ec718ffcce
Image format version: 3.0
Image optimization: Sand-4GB

Uptime: 54 weeks, 5 days, 7 hours and 2 minutes
Total memory: 8051592 kB
Free memory: 5572560 kB
`
	p := &EOSParser{}
	result, err := p.Parse([]byte(input), parse.FilterOpts{})
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}

	// Check metadata
	if result.Metadata["model"] != "DCS-7280SR-48C6-R" {
		t.Errorf("model = %q, want %q", result.Metadata["model"], "DCS-7280SR-48C6-R")
	}
	if result.Metadata["serial"] != "JPE17151856" {
		t.Errorf("serial = %q, want %q", result.Metadata["serial"], "JPE17151856")
	}
	if result.Metadata["version"] != "4.31.6M" {
		t.Errorf("version = %q, want %q", result.Metadata["version"], "4.31.6M")
	}

	// Check first line is Model
	if len(result.Lines) == 0 {
		t.Fatal("no lines produced")
	}
	if !strings.HasPrefix(result.Lines[0], "!Model: ") {
		t.Errorf("first line = %q, want !Model: prefix", result.Lines[0])
	}

	// Check key:value lines exist
	foundSerial := false
	for _, line := range result.Lines {
		if strings.HasPrefix(line, "!Serial number:") {
			foundSerial = true
		}
	}
	if !foundSerial {
		t.Error("missing !Serial number: line")
	}

	// Check Uptime and Free memory are filtered out
	for _, line := range result.Lines {
		if strings.Contains(line, "Uptime") {
			t.Errorf("Uptime line should be filtered: %q", line)
		}
		if strings.Contains(line, "Free memory") {
			t.Errorf("Free memory line should be filtered: %q", line)
		}
	}

	// Check Total memory is present
	foundTotal := false
	for _, line := range result.Lines {
		if strings.Contains(line, "Total memory") {
			foundTotal = true
		}
	}
	if !foundTotal {
		t.Error("missing Total memory line")
	}
}

// ---------------------------------------------------------------------------
// Show Boot Config
// ---------------------------------------------------------------------------

func TestShowBootConfig(t *testing.T) {
	input := `show boot-config
Software image: flash:/EOS-4.31.6M.swi
Console speed: (not set)
`
	p := &EOSParser{}
	result, err := p.Parse([]byte(input), parse.FilterOpts{})
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}

	found := false
	for _, line := range result.Lines {
		if line == "!Software image: flash:/EOS-4.31.6M.swi" {
			found = true
		}
	}
	if !found {
		t.Errorf("missing !Software image line, got: %v", result.Lines)
	}
}

// ---------------------------------------------------------------------------
// Show Boot Extensions
// ---------------------------------------------------------------------------

func TestShowBootExtensions(t *testing.T) {
	input := `show boot-extensions
TerminAttr-1.5.2-1.swix
`
	p := &EOSParser{}
	result, err := p.Parse([]byte(input), parse.FilterOpts{})
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}

	found := false
	for _, line := range result.Lines {
		if line == "!BootExtension: TerminAttr-1.5.2-1.swix" {
			found = true
		}
	}
	if !found {
		t.Errorf("missing !BootExtension line, got: %v", result.Lines)
	}
}

// ---------------------------------------------------------------------------
// Show Extensions
// ---------------------------------------------------------------------------

func TestShowExtensions(t *testing.T) {
	input := `show extensions
Name                         Version/Release      Status      Extension
---------------------------- -------------------- ----------- ---------
TerminAttr-1.5.2-1.swix      v1.5.2/1             A, B        25
`
	p := &EOSParser{}
	result, err := p.Parse([]byte(input), parse.FilterOpts{})
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}

	foundHeader := false
	foundEntry := false
	for _, line := range result.Lines {
		if strings.HasPrefix(line, "!Extensions:") {
			foundHeader = true
		}
		if strings.Contains(line, "TerminAttr") {
			foundEntry = true
		}
	}
	if !foundHeader {
		t.Error("missing !Extensions: header")
	}
	if !foundEntry {
		t.Errorf("missing extension entry, got: %v", result.Lines)
	}
}

// ---------------------------------------------------------------------------
// Show Running Config
// ---------------------------------------------------------------------------

func TestShowRunningConfig(t *testing.T) {
	input := `show running-config
! Command: show running-config
! device: switch1 (DCS-7280SR-48C6, EOS-4.31.6M)
!
! Time: Wed Apr 16 16:00:00 2026
!
no aaa root
!
username admin privilege 15 role network-admin secret sha512 $6$hash
!
snmp-server community public RO
snmp-server host 10.0.0.1 public
!
end
`
	p := &EOSParser{}
	result, err := p.Parse([]byte(input), parse.FilterOpts{NoCommStr: true, FilterPwds: 1})
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}

	for _, line := range result.Lines {
		if strings.Contains(line, "! Command:") {
			t.Errorf("! Command: header should be stripped: %q", line)
		}
		if strings.Contains(line, "! device:") {
			t.Errorf("! device: header should be stripped: %q", line)
		}
		if strings.Contains(line, "! Time:") {
			t.Errorf("! Time: header should be stripped: %q", line)
		}
		if strings.Contains(line, "snmp-server community public") {
			t.Errorf("SNMP community should be filtered: %q", line)
		}
		if strings.Contains(line, "secret sha512") {
			t.Errorf("secret should be filtered: %q", line)
		}
	}

	// Check end marker is present
	if len(result.Lines) > 0 && result.Lines[len(result.Lines)-1] != "end" {
		t.Errorf("last line = %q, want 'end'", result.Lines[len(result.Lines)-1])
	}
}

func TestShowRunningConfigConsecutiveBangs(t *testing.T) {
	input := `show running-config
!
!
!
no aaa root
end
`
	p := &EOSParser{}
	result, err := p.Parse([]byte(input), parse.FilterOpts{})
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}

	bangCount := 0
	for _, line := range result.Lines {
		if line == "!" {
			bangCount++
		}
	}
	if bangCount != 1 {
		t.Errorf("consecutive ! lines should be collapsed to 1, got %d", bangCount)
	}
}

// ---------------------------------------------------------------------------
// Diff Config
// ---------------------------------------------------------------------------

func TestDiffConfigNoChanges(t *testing.T) {
	input := `diff startup-config running-config
`
	p := &EOSParser{}
	result, err := p.Parse([]byte(input), parse.FilterOpts{})
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}

	for _, line := range result.Lines {
		if strings.Contains(line, "unsaved changes") {
			t.Errorf("should not have unsaved changes banner for empty diff: %q", line)
		}
	}
}

func TestDiffConfigWithChanges(t *testing.T) {
	input := `diff startup-config running-config
3c3
< no aaa root
---
> aaa root
`
	p := &EOSParser{}
	result, err := p.Parse([]byte(input), parse.FilterOpts{})
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}

	found := false
	for _, line := range result.Lines {
		if strings.Contains(line, "unsaved changes exist") {
			found = true
		}
	}
	if !found {
		t.Errorf("missing unsaved changes banner, got: %v", result.Lines)
	}
}

// ---------------------------------------------------------------------------
// isCommandHeader
// ---------------------------------------------------------------------------

func TestIsCommandHeader(t *testing.T) {
	tests := []struct {
		line    string
		cmd     string
		expect  bool
	}{
		{"show version", "show version", true},
		{"am6-lfs-a06-p01#show version", "show version", true},
		{"[16:23] am6-lfs-a06-p01#show version", "show version", true},
		{"show boot-config", "show boot-config", true},
		{"show running-config", "show running-config", true},
		{"diff startup-config running-config", "diff startup-config running-config", true},
		{"show version detail", "show version", false},
		{"Arista DCS-7280SR-48C6-R", "show version", false},
	}

	for _, tc := range tests {
		got := isCommandHeader(tc.line, tc.cmd)
		if got != tc.expect {
			t.Errorf("isCommandHeader(%q, %q) = %v, want %v", tc.line, tc.cmd, got, tc.expect)
		}
	}
}

// ---------------------------------------------------------------------------
// Full integration test
// ---------------------------------------------------------------------------

func TestFullIntegration(t *testing.T) {
	input := `show version
Arista DCS-7280SR-48C6-R
Hardware version: 10.01
Serial number: JPE17151856
Software image version: 4.31.6M

Uptime: 54 weeks, 5 days, 7 hours and 2 minutes
Total memory: 8051592 kB
Free memory: 5572560 kB
show boot-config
Software image: flash:/EOS-4.31.6M.swi
show boot-extensions
TerminAttr-1.5.2-1.swix
show running-config
! Command: show running-config
! device: switch1 (DCS-7280SR-48C6, EOS-4.31.6M)
!
no aaa root
!
end
`
	p := &EOSParser{}
	result, err := p.Parse([]byte(input), parse.FilterOpts{})
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}

	if len(result.Lines) == 0 {
		t.Fatal("expected output lines, got none")
	}

	// Check first line is Model
	if !strings.HasPrefix(result.Lines[0], "!Model: Arista") {
		t.Errorf("first line = %q, want !Model: Arista prefix", result.Lines[0])
	}

	// Check metadata
	if result.Metadata["model"] != "DCS-7280SR-48C6-R" {
		t.Errorf("model = %q", result.Metadata["model"])
	}
	if result.Metadata["serial"] != "JPE17151856" {
		t.Errorf("serial = %q", result.Metadata["serial"])
	}
	if result.Metadata["version"] != "4.31.6M" {
		t.Errorf("version = %q", result.Metadata["version"])
	}

	// Check last line is end
	last := result.Lines[len(result.Lines)-1]
	if last != "end" {
		t.Errorf("last line = %q, want 'end'", last)
	}
}

// ---------------------------------------------------------------------------
// Empty input
// ---------------------------------------------------------------------------

func TestEmptyInput(t *testing.T) {
	p := &EOSParser{}
	result, err := p.Parse([]byte(""), parse.FilterOpts{})
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}
	if len(result.Lines) != 0 {
		t.Errorf("expected 0 lines for empty input, got %d", len(result.Lines))
	}
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func compileRegexp(t *testing.T, pattern string) *regexp.Regexp {
	t.Helper()
	re, err := regexp.Compile(pattern)
	if err != nil {
		t.Fatalf("failed to compile pattern %q: %v", pattern, err)
	}
	return re
}

// Verify DeviceOpts implements the interface
func TestDeviceOptsImplementsInterface(t *testing.T) {
	var _ connect.DeviceOpts = (&EOSParser{}).DeviceOpts()
}