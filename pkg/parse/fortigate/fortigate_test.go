package fortigate

import (
	"regexp"
	"testing"

	"gorancid/pkg/parse"
)

// ---------------------------------------------------------------------------
// Test data — realistic FortiGate output
// ---------------------------------------------------------------------------

// Realistic FortiGate "get system status" output
const systemStatusOutput = `get system status
Version: FortiGate-100F v7.4.2,build2571,231212 (GA.FGA)
Virus-DB: 91.00960(2024-01-15 08:42)
IPS-DB: 6.00741(2024-01-15 01:54)
IPS-ETDB: 6.00741(2024-01-15 01:54)
APP-DB: 6.00741(2024-01-15 01:54)
industrial-db: 6.00741(2024-01-15 01:54)
Botnet DB: 1.00000(2024-01-12 18:04)
Extended DB: 1.00000(2024-01-12 18:04)
AV AI/ML Model: 1.00000(2024-01-12 18:04)
IPS Malicious URL Database: 1.00000(2024-01-12 18:04)
Proxy-APP-DB: 6.00741(2024-01-15 01:54)
Proxy-IPS-ETDB: 6.00741(2024-01-15 01:54)
Serial-Number: FGT100FTK12345678
Hostname: HQ-FW-01
System time: 2024-01-16 10:30:00
Cluster uptime:  45 days, 12 hours, 30 minutes
FortiClient application signature package: 6.00741
License Status: Valid
`

// Realistic FortiGate "show" output (abbreviated but representative)
// FortiGate uses "set password ENC <base64value>" format for encrypted passwords
const showConfOutput = `show
#conf_file_ver=3858714265.0
!System time: 2024-01-16 10:30:00
config system global
    set hostname "HQ-FW-01"
    set admin-sport 443
end
config system admin
    edit "admin"
        set password ENC AQixy1Tf8Jk7nV2wP0sR3g==
        set last-login 1705398600
    next
end
config user local
    edit "vpnuser"
        set type password
        set password ENC BQ8xk2Tm5Np9oW3qR1vY4z==
    next
end
config vpn certificate local
    edit "Fortinet_CA_SSL"
        set md5-key abcd1234efgh5678
    next
    edit "RSA_Server_Key"
        set private-key "-----BEGIN RSA PRIVATE KEY-----
MIIEpAIBAAKCAQEA0Z3VS5JJcds3xfn/ygWyF8PbnGy4JI4W0bDR0cD1aK6IjQdU
mXSr2R5pL6pJ9V3yN3kG2hM7pT1oJ8vE4wR6uK2sL5nF7pG3qH9vB1cT4mD8fK
-----END RSA PRIVATE KEY-----
"
    next
end
config system interface
    edit "port1"
        set ip 10.0.0.1 255.255.255.0
    next
end
`

// Full combined output simulating both commands
var fullOutput = systemStatusOutput + "\n" + showConfOutput

// ---------------------------------------------------------------------------
// Metadata extraction tests
// ---------------------------------------------------------------------------

func TestMetadataExtraction(t *testing.T) {
	p := &FortiGateParser{}
	cfg, err := p.Parse([]byte(fullOutput), parse.FilterOpts{})
	if err != nil {
		t.Fatalf("Parse returned error: %v", err)
	}

	tests := []struct {
		key, want string
	}{
		{"version", "FortiGate-100F v7.4.2,build2571,231212 (GA.FGA)"},
		{"serial", "FGT100FTK12345678"},
		{"hostname", "HQ-FW-01"},
		{"model", "100F"},
	}

	for _, tc := range tests {
		got, ok := cfg.Metadata[tc.key]
		if !ok {
			t.Errorf("metadata key %q not found", tc.key)
			continue
		}
		if got != tc.want {
			t.Errorf("metadata[%q] = %q, want %q", tc.key, got, tc.want)
		}
	}
}

func TestDeviceOptsPromptPatternMatchesInteractivePrompts(t *testing.T) {
	p := &FortiGateParser{}
	opts := p.DeviceOpts()
	re, err := regexp.Compile(opts.PromptPattern)
	if err != nil {
		t.Fatalf("PromptPattern does not compile: %v", err)
	}

	prompts := []string{
		"dc2-fw-mgmt-p01 $",
		"\ndc2-fw-mgmt-p01 $",
		"\ndc2-fw-mgmt-p01 #",
		"\ndc2-fw-mgmt-p01 (global) #",
		"\ndc2-fw-mgmt-p01 (interface) $",
	}
	for _, prompt := range prompts {
		if !re.MatchString(prompt) {
			t.Fatalf("prompt pattern %q did not match %q", opts.PromptPattern, prompt)
		}
	}
}

// ---------------------------------------------------------------------------
// System status filtering tests
// ---------------------------------------------------------------------------

func TestSystemTimeAlwaysSuppressed(t *testing.T) {
	p := &FortiGateParser{}
	cfg, err := p.Parse([]byte(systemStatusOutput), parse.FilterOpts{})
	if err != nil {
		t.Fatalf("Parse returned error: %v", err)
	}

	for _, line := range cfg.Lines {
		if matched, _ := regexp.MatchString(`system time:`, line); matched {
			t.Errorf("system time line should be suppressed, got: %q", line)
		}
	}
}

func TestClusterUptimeAlwaysSuppressed(t *testing.T) {
	p := &FortiGateParser{}
	cfg, err := p.Parse([]byte(systemStatusOutput), parse.FilterOpts{})
	if err != nil {
		t.Fatalf("Parse returned error: %v", err)
	}

	for _, line := range cfg.Lines {
		if matched, _ := regexp.MatchString(`Cluster uptime:`, line); matched {
			t.Errorf("Cluster uptime line should be suppressed, got: %q", line)
		}
	}
}

func TestFortiClientSigAlwaysSuppressed(t *testing.T) {
	p := &FortiGateParser{}
	cfg, err := p.Parse([]byte(systemStatusOutput), parse.FilterOpts{})
	if err != nil {
		t.Fatalf("Parse returned error: %v", err)
	}

	for _, line := range cfg.Lines {
		if matched, _ := regexp.MatchString(`FortiClient application signature package:`, line); matched {
			t.Errorf("FortiClient signature line should be suppressed, got: %q", line)
		}
	}
}

// ---------------------------------------------------------------------------
// Signature DB filtering tests
// ---------------------------------------------------------------------------

func TestSigDBPassThroughAtFilterOsc0(t *testing.T) {
	p := &FortiGateParser{}
	cfg, err := p.Parse([]byte(systemStatusOutput), parse.FilterOpts{FilterOsc: 0})
	if err != nil {
		t.Fatalf("Parse returned error: %v", err)
	}

	sigDBNames := []string{"Virus-DB", "IPS-DB", "IPS-ETDB", "APP-DB", "industrial-db",
		"Botnet DB", "Extended DB", "AV AI/ML Model", "IPS Malicious URL Database",
		"Proxy-APP-DB", "Proxy-IPS-ETDB"}

	for _, name := range sigDBNames {
		found := false
		for _, line := range cfg.Lines {
			if matched, _ := regexp.MatchString(regexp.QuoteMeta(name), line); matched {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("at FilterOsc=0, %q line should pass through but was filtered", name)
		}
	}
}

func TestSigDBFilteredAtFilterOsc2(t *testing.T) {
	p := &FortiGateParser{}
	cfg, err := p.Parse([]byte(systemStatusOutput), parse.FilterOpts{FilterOsc: 2})
	if err != nil {
		t.Fatalf("Parse returned error: %v", err)
	}

	sigDBNames := []string{"Virus-DB", "IPS-DB", "IPS-ETDB", "APP-DB", "industrial-db",
		"Botnet DB", "Extended DB", "AV AI/ML Model", "IPS Malicious URL Database",
		"Proxy-APP-DB", "Proxy-IPS-ETDB"}

	for _, name := range sigDBNames {
		for _, line := range cfg.Lines {
			if matched, _ := regexp.MatchString(regexp.QuoteMeta(name), line); matched {
				t.Errorf("at FilterOsc=2, %q line should be filtered but got: %q", name, line)
			}
		}
	}
}

func TestSigDBPassThroughAtFilterOsc1(t *testing.T) {
	p := &FortiGateParser{}
	cfg, err := p.Parse([]byte(systemStatusOutput), parse.FilterOpts{FilterOsc: 1})
	if err != nil {
		t.Fatalf("Parse returned error: %v", err)
	}

	// At FilterOsc=1, signature DB lines should still pass through (only filtered at >=2)
	sigDBNames := []string{"Virus-DB", "IPS-DB", "APP-DB"}
	for _, name := range sigDBNames {
		found := false
		for _, line := range cfg.Lines {
			if matched, _ := regexp.MatchString(regexp.QuoteMeta(name), line); matched {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("at FilterOsc=1, %q line should pass through but was filtered", name)
		}
	}
}

// ---------------------------------------------------------------------------
// Show configuration filtering tests
// ---------------------------------------------------------------------------

func TestConfSystemTimeRemoved(t *testing.T) {
	p := &FortiGateParser{}
	cfg, err := p.Parse([]byte(showConfOutput), parse.FilterOpts{})
	if err != nil {
		t.Fatalf("Parse returned error: %v", err)
	}

	for _, line := range cfg.Lines {
		if matched, _ := regexp.MatchString(`!System time:`, line); matched {
			t.Errorf("!System time: line should be removed, got: %q", line)
		}
	}
}

func TestConfFileVerRemoved(t *testing.T) {
	p := &FortiGateParser{}
	cfg, err := p.Parse([]byte(showConfOutput), parse.FilterOpts{})
	if err != nil {
		t.Fatalf("Parse returned error: %v", err)
	}

	for _, line := range cfg.Lines {
		if matched, _ := regexp.MatchString(`conf_file_ver=`, line); matched {
			t.Errorf("conf_file_ver= line should be removed, got: %q", line)
		}
	}
}

// ---------------------------------------------------------------------------
// Password/encryption filtering tests
// ---------------------------------------------------------------------------

func TestEncPasswordFilteredWithFilterPwds(t *testing.T) {
	p := &FortiGateParser{}
	cfg, err := p.Parse([]byte(showConfOutput), parse.FilterOpts{FilterPwds: 1})
	if err != nil {
		t.Fatalf("Parse returned error: %v", err)
	}

	for _, line := range cfg.Lines {
		if matched, _ := regexp.MatchString(`AQixy1Tf8Jk7nV2wP0sR3g==`, line); matched {
			t.Errorf("enc password value should be filtered with FilterPwds=1, got: %q", line)
		}
		if matched, _ := regexp.MatchString(`BQ8xk2Tm5Np9oW3qR1vY4z==`, line); matched {
			t.Errorf("enc password value should be filtered with FilterPwds=1, got: %q", line)
		}
	}
}

func TestEncPasswordFilteredWithFilterOsc(t *testing.T) {
	p := &FortiGateParser{}
	cfg, err := p.Parse([]byte(showConfOutput), parse.FilterOpts{FilterOsc: 1})
	if err != nil {
		t.Fatalf("Parse returned error: %v", err)
	}

	for _, line := range cfg.Lines {
		if matched, _ := regexp.MatchString(`AQixy1Tf8Jk7nV2wP0sR3g==`, line); matched {
			t.Errorf("enc password value should be filtered with FilterOsc=1, got: %q", line)
		}
	}
}

func TestEncPasswordNotFilteredWithNoFilter(t *testing.T) {
	p := &FortiGateParser{}
	cfg, err := p.Parse([]byte(showConfOutput), parse.FilterOpts{})
	if err != nil {
		t.Fatalf("Parse returned error: %v", err)
	}

	found := false
	for _, line := range cfg.Lines {
		if matched, _ := regexp.MatchString(`AQixy1Tf8Jk7nV2wP0sR3g==`, line); matched {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("enc password value should pass through with no filtering, but was filtered")
	}
}

func TestEncPasswordReplacementFormat(t *testing.T) {
	p := &FortiGateParser{}
	cfg, err := p.Parse([]byte(showConfOutput), parse.FilterOpts{FilterPwds: 1})
	if err != nil {
		t.Fatalf("Parse returned error: %v", err)
	}

	// Check that the replacement format is correct: "ENC <removed>"
	found := false
	for _, line := range cfg.Lines {
		if matched, _ := regexp.MatchString(`ENC <removed>`, line); matched {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected enc password replacement format 'ENC <removed>', not found in output")
	}
}

// ---------------------------------------------------------------------------
// Last-login filtering tests
// ---------------------------------------------------------------------------

func TestLastLoginFilteredWithFilterOsc(t *testing.T) {
	p := &FortiGateParser{}
	cfg, err := p.Parse([]byte(showConfOutput), parse.FilterOpts{FilterOsc: 1})
	if err != nil {
		t.Fatalf("Parse returned error: %v", err)
	}

	for _, line := range cfg.Lines {
		if matched, _ := regexp.MatchString(`last-login 1705398600`, line); matched {
			t.Errorf("last-login value should be filtered with FilterOsc=1, got: %q", line)
		}
	}
}

func TestLastLoginNotFilteredWithoutFilterOsc(t *testing.T) {
	p := &FortiGateParser{}
	cfg, err := p.Parse([]byte(showConfOutput), parse.FilterOpts{FilterPwds: 1, FilterOsc: 0})
	if err != nil {
		t.Fatalf("Parse returned error: %v", err)
	}

	found := false
	for _, line := range cfg.Lines {
		if matched, _ := regexp.MatchString(`last-login 1705398600`, line); matched {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("last-login value should pass through with FilterOsc=0, but was filtered")
	}
}

// ---------------------------------------------------------------------------
// Private key filtering tests
// ---------------------------------------------------------------------------

func TestRSAPrivateKeyFilteredWithFilterOsc(t *testing.T) {
	p := &FortiGateParser{}
	cfg, err := p.Parse([]byte(showConfOutput), parse.FilterOpts{FilterOsc: 1})
	if err != nil {
		t.Fatalf("Parse returned error: %v", err)
	}

	for _, line := range cfg.Lines {
		if matched, _ := regexp.MatchString(`BEGIN RSA PRIVATE KEY`, line); matched {
			t.Errorf("RSA private key BEGIN line should be replaced with FilterOsc=1, got: %q", line)
		}
		if matched, _ := regexp.MatchString(`END RSA PRIVATE KEY`, line); matched {
			t.Errorf("RSA private key END line should be removed with FilterOsc=1, got: %q", line)
		}
		if matched, _ := regexp.MatchString(`MIIEpAIBAAKCAQEA`, line); matched {
			t.Errorf("RSA key body should not appear with FilterOsc=1, got: %q", line)
		}
	}
}

func TestRSAPrivateKeyReplacedWithRemoved(t *testing.T) {
	p := &FortiGateParser{}
	cfg, err := p.Parse([]byte(showConfOutput), parse.FilterOpts{FilterOsc: 1})
	if err != nil {
		t.Fatalf("Parse returned error: %v", err)
	}

	found := false
	for _, line := range cfg.Lines {
		if line == "<removed>" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected '<removed>' replacement for RSA private key block, not found in output")
	}
}

func TestRSAPrivateKeyNotFilteredWithoutFilterOsc(t *testing.T) {
	p := &FortiGateParser{}
	cfg, err := p.Parse([]byte(showConfOutput), parse.FilterOpts{FilterOsc: 0})
	if err != nil {
		t.Fatalf("Parse returned error: %v", err)
	}

	found := false
	for _, line := range cfg.Lines {
		if matched, _ := regexp.MatchString(`BEGIN RSA PRIVATE KEY`, line); matched {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("RSA private key should pass through with FilterOsc=0, but was filtered")
	}
}

// ---------------------------------------------------------------------------
// md5-key filtering tests
// ---------------------------------------------------------------------------

func TestMD5KeyFilteredWithFilterOsc(t *testing.T) {
	p := &FortiGateParser{}
	cfg, err := p.Parse([]byte(showConfOutput), parse.FilterOpts{FilterOsc: 1})
	if err != nil {
		t.Fatalf("Parse returned error: %v", err)
	}

	for _, line := range cfg.Lines {
		if matched, _ := regexp.MatchString(`md5-key abcd1234efgh5678`, line); matched {
			t.Errorf("md5-key value should be filtered with FilterOsc=1, got: %q", line)
		}
	}
}

func TestMD5KeyReplacementFormat(t *testing.T) {
	p := &FortiGateParser{}
	cfg, err := p.Parse([]byte(showConfOutput), parse.FilterOpts{FilterOsc: 1})
	if err != nil {
		t.Fatalf("Parse returned error: %v", err)
	}

	found := false
	for _, line := range cfg.Lines {
		if matched, _ := regexp.MatchString(`md5-key <removed>`, line); matched {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected md5-key replacement format 'md5-key <removed>', not found in output")
	}
}

func TestMD5KeyNotFilteredWithoutFilterOsc(t *testing.T) {
	p := &FortiGateParser{}
	cfg, err := p.Parse([]byte(showConfOutput), parse.FilterOpts{FilterOsc: 0})
	if err != nil {
		t.Fatalf("Parse returned error: %v", err)
	}

	found := false
	for _, line := range cfg.Lines {
		if matched, _ := regexp.MatchString(`md5-key abcd1234efgh5678`, line); matched {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("md5-key value should pass through with FilterOsc=0, but was filtered")
	}
}

// ---------------------------------------------------------------------------
// DeviceOpts test
// ---------------------------------------------------------------------------

func TestDeviceOpts(t *testing.T) {
	p := &FortiGateParser{}
	opts := p.DeviceOpts()

	if opts.DeviceType != "fortigate" {
		t.Errorf("DeviceType = %q, want %q", opts.DeviceType, "fortigate")
	}
	if opts.PromptPattern != `(?:^|[\r\n])[^\r\n]*[#\$]\s*$` {
		t.Errorf("PromptPattern = %q, unexpected value", opts.PromptPattern)
	}
	if len(opts.SetupCommands) != 3 {
		t.Errorf("len(SetupCommands) = %d, want 3", len(opts.SetupCommands))
	}
	if opts.EnableCmd != "" {
		t.Errorf("EnableCmd = %q, want empty", opts.EnableCmd)
	}
	if opts.DisablePagingCmd != "config system console\nset output standard\nend" {
		t.Errorf("DisablePagingCmd = %q, unexpected value", opts.DisablePagingCmd)
	}
}

// ---------------------------------------------------------------------------
// Parser registration test
// ---------------------------------------------------------------------------

func TestParserRegistered(t *testing.T) {
	p, ok := parse.Lookup("fortigate")
	if !ok {
		t.Fatal("fortigate parser not registered")
	}
	if _, ok = p.(*FortiGateParser); !ok {
		t.Fatal("fortigate parser is not a *FortiGateParser")
	}
}

// ---------------------------------------------------------------------------
// Full integration test
// ---------------------------------------------------------------------------

func TestFullParseNoFilter(t *testing.T) {
	p := &FortiGateParser{}
	cfg, err := p.Parse([]byte(fullOutput), parse.FilterOpts{})
	if err != nil {
		t.Fatalf("Parse returned error: %v", err)
	}

	// Metadata should be extracted
	if cfg.Metadata["version"] == "" {
		t.Error("version metadata should be extracted")
	}
	if cfg.Metadata["serial"] == "" {
		t.Error("serial metadata should be extracted")
	}
	if cfg.Metadata["hostname"] == "" {
		t.Error("hostname metadata should be extracted")
	}
	if cfg.Metadata["model"] == "" {
		t.Error("model metadata should be extracted")
	}

	// System time should always be removed
	for _, line := range cfg.Lines {
		if matched, _ := regexp.MatchString(`system time:`, line); matched {
			t.Errorf("system time should always be suppressed, got: %q", line)
		}
	}

	// conf_file_ver should always be removed
	for _, line := range cfg.Lines {
		if matched, _ := regexp.MatchString(`conf_file_ver=`, line); matched {
			t.Errorf("conf_file_ver should always be suppressed, got: %q", line)
		}
	}

	// With no filtering, passwords should pass through
	found := false
	for _, line := range cfg.Lines {
		if matched, _ := regexp.MatchString(`AQixy1Tf8Jk7nV2wP0sR3g==`, line); matched {
			found = true
			break
		}
	}
	if !found {
		t.Error("enc password should pass through with no filtering")
	}
}

func TestFullParseWithMaxFilter(t *testing.T) {
	p := &FortiGateParser{}
	cfg, err := p.Parse([]byte(fullOutput), parse.FilterOpts{FilterPwds: 2, FilterOsc: 2})
	if err != nil {
		t.Fatalf("Parse returned error: %v", err)
	}

	// Metadata should be extracted
	if cfg.Metadata["version"] == "" {
		t.Error("version metadata should be extracted")
	}
	if cfg.Metadata["serial"] == "" {
		t.Error("serial metadata should be extracted")
	}

	// Signature DB lines should be filtered at FilterOsc=2
	sigDBNames := []string{"Virus-DB", "IPS-DB", "APP-DB", "industrial-db"}
	for _, name := range sigDBNames {
		for _, line := range cfg.Lines {
			if matched, _ := regexp.MatchString(regexp.QuoteMeta(name), line); matched {
				t.Errorf("at FilterOsc=2, %q line should be filtered, got: %q", name, line)
			}
		}
	}

	// Passwords should be replaced
	for _, line := range cfg.Lines {
		if matched, _ := regexp.MatchString(`AQixy1Tf8Jk7nV2wP0sR3g==`, line); matched {
			t.Errorf("enc password should be filtered at FilterPwds=2, got: %q", line)
		}
	}

	// Private keys should be replaced
	for _, line := range cfg.Lines {
		if matched, _ := regexp.MatchString(`BEGIN RSA PRIVATE KEY`, line); matched {
			t.Errorf("RSA private key should be replaced at FilterOsc=2, got: %q", line)
		}
	}

	// md5-key should be replaced
	for _, line := range cfg.Lines {
		if matched, _ := regexp.MatchString(`md5-key abcd1234efgh5678`, line); matched {
			t.Errorf("md5-key should be filtered at FilterOsc=2, got: %q", line)
		}
	}

	// Normal config lines should pass through
	found := false
	for _, line := range cfg.Lines {
		if matched, _ := regexp.MatchString(`set admin-sport 443`, line); matched {
			found = true
			break
		}
	}
	if !found {
		t.Error("normal config lines should pass through even with max filtering")
	}
}

// ---------------------------------------------------------------------------
// Edge cases
// ---------------------------------------------------------------------------

func TestEmptyInput(t *testing.T) {
	p := &FortiGateParser{}
	cfg, err := p.Parse([]byte(""), parse.FilterOpts{})
	if err != nil {
		t.Fatalf("Parse returned error: %v", err)
	}
	if len(cfg.Lines) != 0 {
		t.Errorf("expected 0 lines for empty input, got %d", len(cfg.Lines))
	}
}

func TestUnknownSectionPassesThrough(t *testing.T) {
	p := &FortiGateParser{}
	input := "some random output\nmore output\n"
	cfg, err := p.Parse([]byte(input), parse.FilterOpts{})
	if err != nil {
		t.Fatalf("Parse returned error: %v", err)
	}
	// Lines outside recognized sections are not included (section is unknown)
	if len(cfg.Lines) != 0 {
		t.Errorf("expected 0 lines for unrecognized sections, got %d", len(cfg.Lines))
	}
}

func TestModelExtractionFromVersionLine(t *testing.T) {
	p := &FortiGateParser{}
	input := `get system status
Version: FortiGate-200F v7.2.8,build1639,231108 (GA.FGA)
Serial-Number: FGT200FTK87654321
Hostname: DC-FW-02
`
	cfg, err := p.Parse([]byte(input), parse.FilterOpts{})
	if err != nil {
		t.Fatalf("Parse returned error: %v", err)
	}

	if cfg.Metadata["model"] != "200F" {
		t.Errorf("model = %q, want %q", cfg.Metadata["model"], "200F")
	}
}

func TestPrivateKeyBlockCompletelySuppressed(t *testing.T) {
	p := &FortiGateParser{}
	// Test with a standalone private key block (not inside a set command)
	input := `show
config vpn certificate local
    edit "TestKey"
        set private-key "-----BEGIN RSA PRIVATE KEY-----
MIIEpAIBAAKCAQEA0Z3VS5JJcds3xfn/ygWyF8PbnGy4JI4W0bDR0cD1aK6IjQdU
mXSr2R5pL6pJ9V3yN3kG2hM7pT1oJ8vE4wR6uK2sL5nF7pG3qH9vB1cT4mD8fK
-----END RSA PRIVATE KEY-----
"
    next
end
`
	cfg, err := p.Parse([]byte(input), parse.FilterOpts{FilterOsc: 1})
	if err != nil {
		t.Fatalf("Parse returned error: %v", err)
	}

	// Verify private key body lines are completely suppressed
	for _, line := range cfg.Lines {
		if matched, _ := regexp.MatchString(`MIIEpAIBAAKCAQEA`, line); matched {
			t.Errorf("private key body should be suppressed, got: %q", line)
		}
		if matched, _ := regexp.MatchString(`BEGIN RSA PRIVATE KEY`, line); matched {
			t.Errorf("private key BEGIN marker should be suppressed, got: %q", line)
		}
		if matched, _ := regexp.MatchString(`END RSA PRIVATE KEY`, line); matched {
			t.Errorf("private key END marker should be suppressed, got: %q", line)
		}
	}

	// Verify "<removed>" appears exactly once for the block
	removedCount := 0
	for _, line := range cfg.Lines {
		if line == "<removed>" {
			removedCount++
		}
	}
	if removedCount != 1 {
		t.Errorf("expected exactly 1 '<removed>' for private key block, got %d", removedCount)
	}
}
