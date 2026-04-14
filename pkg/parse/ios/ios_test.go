package ios

import (
	"os"
	"strings"
	"testing"

	"gorancid/pkg/parse"
)

func loadTestdata(t *testing.T, name string) []byte {
	t.Helper()
	data, err := os.ReadFile("testdata/" + name)
	if err != nil {
		t.Fatalf("reading testdata/%s: %v", name, err)
	}
	return data
}

// ---------------------------------------------------------------------------
// Registration
// ---------------------------------------------------------------------------

func TestRegistered(t *testing.T) {
	p, ok := parse.Lookup("ios")
	if !ok {
		t.Fatal("ios parser not registered")
	}
	if _, ok := p.(*IOSParser); !ok {
		t.Fatal("ios parser is not an IOSParser")
	}
}

func TestRegisteredAliases(t *testing.T) {
	for _, deviceType := range []string{"cisco", "cat5k"} {
		p, ok := parse.Lookup(deviceType)
		if !ok {
			t.Fatalf("%s parser alias not registered", deviceType)
		}
		if _, ok := p.(*IOSParser); !ok {
			t.Fatalf("%s parser alias is not an IOSParser", deviceType)
		}
	}
}

// ---------------------------------------------------------------------------
// ShowVersion metadata extraction
// ---------------------------------------------------------------------------

func TestShowVersionMetadata(t *testing.T) {
	data := loadTestdata(t, "show_version.txt")
	p := &IOSParser{}
	cfg, err := p.Parse(data, parse.FilterOpts{})
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}

	tests := []struct {
		key, want string
	}{
		{"image", "C7200-ADVENTERPRISEK9-M"},
		{"version", "15.2(4)S5, RELEASE SOFTWARE (fc1)"},
		{"processor", "7206VXR"},
		{"cpu", "NPE-G1"},
		{"memory", "524288k"},
		{"serial", "FTX1234ABCD"},
		{"config_register", "0x2102"},
		{"boot_image", "flash:c7200-adventerprisek9-mz.152-4.S5.bin"},
	}
	for _, tc := range tests {
		got := cfg.Metadata[tc.key]
		if got != tc.want {
			t.Errorf("metadata[%q] = %q, want %q", tc.key, got, tc.want)
		}
	}
}

func TestShowVersionFiltersVolatile(t *testing.T) {
	data := loadTestdata(t, "show_version.txt")
	p := &IOSParser{}
	cfg, err := p.Parse(data, parse.FilterOpts{})
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}

	for _, line := range cfg.Lines {
		if strings.HasPrefix(line, "Load for five") {
			t.Error("Load for five line should be filtered")
		}
		if strings.HasPrefix(line, "Time source is") {
			t.Error("Time source line should be filtered")
		}
		if strings.Contains(line, "<--- More --->") || strings.Contains(line, "<-- More -->") {
			t.Error("Pager output should be filtered")
		}
		if strings.TrimSpace(line) == "" {
			t.Error("Empty lines should be filtered from show version")
		}
	}
}

// ---------------------------------------------------------------------------
// WriteTerm filtering
// ---------------------------------------------------------------------------

func TestWriteTermRemovesHeaders(t *testing.T) {
	data := loadTestdata(t, "show_running_config.txt")
	p := &IOSParser{}
	cfg, err := p.Parse(data, parse.FilterOpts{})
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}

	for _, line := range cfg.Lines {
		if strings.HasPrefix(line, "Building configuration") {
			t.Error("Building configuration line should be removed")
		}
		if strings.HasPrefix(line, "Current configuration") {
			t.Error("Current configuration line should be removed")
		}
	}
}

func TestWriteTermRemovesTimestamps(t *testing.T) {
	data := loadTestdata(t, "show_running_config.txt")
	p := &IOSParser{}
	cfg, err := p.Parse(data, parse.FilterOpts{})
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}

	for _, line := range cfg.Lines {
		if strings.Contains(line, "Last configuration change") {
			t.Error("Last configuration change line should be removed")
		}
		if strings.HasPrefix(line, ": Written by") {
			t.Error("Written by line should be removed")
		}
		if strings.HasPrefix(line, ": Saved") {
			t.Error("Saved line should be removed")
		}
		if strings.Contains(line, "no configuration change since last restart") {
			t.Error("no configuration change line should be removed")
		}
	}
}

func TestWriteTermRemovesNtpClockPeriod(t *testing.T) {
	data := loadTestdata(t, "show_running_config.txt")
	p := &IOSParser{}
	cfg, err := p.Parse(data, parse.FilterOpts{})
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}

	for _, line := range cfg.Lines {
		if strings.Contains(line, "ntp clock-period") {
			t.Error("ntp clock-period line should be removed (oscillating)")
		}
	}
}

func TestWriteTermRemovesTftpFlash(t *testing.T) {
	data := loadTestdata(t, "show_running_config.txt")
	p := &IOSParser{}
	cfg, err := p.Parse(data, parse.FilterOpts{})
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}

	for _, line := range cfg.Lines {
		if strings.Contains(line, "tftp-server flash") {
			t.Error("tftp-server flash line should be removed")
		}
	}
}

func TestWriteTermRemovesClockrate(t *testing.T) {
	data := loadTestdata(t, "show_running_config.txt")
	p := &IOSParser{}
	cfg, err := p.Parse(data, parse.FilterOpts{})
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}

	for _, line := range cfg.Lines {
		if strings.Contains(line, "clockrate") {
			t.Error("clockrate line should be removed from serial interfaces")
		}
	}
}

func TestWriteTermRemovesFairQueue(t *testing.T) {
	data := loadTestdata(t, "show_running_config.txt")
	p := &IOSParser{}
	cfg, err := p.Parse(data, parse.FilterOpts{})
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}

	for _, line := range cfg.Lines {
		if strings.Contains(line, "fair-queue individual-limit") {
			t.Error("fair-queue individual-limit line should be removed")
		}
	}
}

// ---------------------------------------------------------------------------
// Password filtering
// ---------------------------------------------------------------------------

func TestPasswordFilterLevel0(t *testing.T) {
	data := loadTestdata(t, "show_running_config.txt")
	p := &IOSParser{}
	cfg, err := p.Parse(data, parse.FilterOpts{FilterPwds: 0})
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}

	// At level 0, passwords should be preserved
	found := false
	for _, line := range cfg.Lines {
		if strings.Contains(line, "enable password cisco123") {
			found = true
		}
	}
	if !found {
		t.Error("Level 0: enable password should be preserved")
	}
}

func TestPasswordFilterLevel1(t *testing.T) {
	data := loadTestdata(t, "show_running_config.txt")
	p := &IOSParser{}
	cfg, err := p.Parse(data, parse.FilterOpts{FilterPwds: 1})
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}

	// Level 1: clear-text passwords should be filtered
	for _, line := range cfg.Lines {
		if strings.Contains(line, "enable password cisco123") {
			t.Error("Level 1: enable password should be filtered")
		}
		if strings.Contains(line, "username admin password 0 adminpass") {
			t.Error("Level 1: username password should be filtered")
		}
		if strings.Contains(line, "password conpass") {
			t.Error("Level 1: line password should be filtered")
		}
		if strings.Contains(line, "password auxpass") {
			t.Error("Level 1: line password should be filtered")
		}
		if strings.Contains(line, "password vtypass") {
			t.Error("Level 1: line password should be filtered")
		}
		if strings.Contains(line, "ip ospf authentication-key 7 0822455D0A16") {
			t.Error("Level 1: OSPF authentication-key should be filtered")
		}
		if strings.Contains(line, "ip ospf message-digest-key 1 md5 7 0822455D0A161C4B") {
			t.Error("Level 1: OSPF message-digest-key should be filtered")
		}
		if strings.Contains(line, "standby 1 authentication cisco") {
			t.Error("Level 1: HSRP authentication should be filtered")
		}
		if strings.Contains(line, "neighbor 10.0.0.2 password 7 0822455D0A161C4B") {
			t.Error("Level 1: BGP neighbor password should be filtered")
		}
		if strings.Contains(line, "ip ftp password ftp123") {
			t.Error("Level 1: FTP password should be filtered")
		}
		if strings.Contains(line, "crypto isakmp key MyIsakmpKey") {
			t.Error("Level 1: ISAKMP key should be filtered")
		}
		if strings.Contains(line, "isis password isispass") {
			t.Error("Level 1: ISIS password should be filtered")
		}
		if strings.Contains(line, "domain-password domainpass") {
			t.Error("Level 1: domain-password should be filtered")
		}
		if strings.Contains(line, "area-password areapass") {
			t.Error("Level 1: area-password should be filtered")
		}
		if strings.Contains(line, "key-string secretkeyval") {
			t.Error("Level 1: key-string should be filtered")
		}
		if strings.Contains(line, "failover key failoverkey") {
			t.Error("Level 1: failover key should be filtered")
		}
		if strings.Contains(line, "cable shared-secret cablesecret") {
			t.Error("Level 1: cable shared-secret should be filtered")
		}
		if strings.Contains(line, "ppp pap sent-username remote password 7 0822455D0A16") {
			t.Error("Level 1: PPP password should be filtered")
		}
	}

	// Level 1: secrets should be preserved
	secretFound := false
	for _, line := range cfg.Lines {
		if strings.Contains(line, "enable secret 5") {
			secretFound = true
		}
		if strings.Contains(line, "username admin secret 5") {
			secretFound = true
		}
	}
	if !secretFound {
		t.Error("Level 1: enable secret and username secret should be preserved")
	}
}

func TestPasswordFilterLevel2(t *testing.T) {
	data := loadTestdata(t, "show_running_config.txt")
	p := &IOSParser{}
	cfg, err := p.Parse(data, parse.FilterOpts{FilterPwds: 2})
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}

	// Level 2: both clear-text and secrets should be filtered
	for _, line := range cfg.Lines {
		if strings.Contains(line, "enable password cisco123") {
			t.Error("Level 2: enable password should be filtered")
		}
		if strings.Contains(line, "enable secret 5 $1$abcde$efghijkLMNOPQRsTUVwxy") {
			t.Error("Level 2: enable secret should be filtered")
		}
		if strings.Contains(line, "username admin password 0 adminpass") {
			t.Error("Level 2: username password should be filtered")
		}
		if strings.Contains(line, "username admin secret 5 $1$abcde$efghijkLMNOPQRsTUVwxy2") {
			t.Error("Level 2: username secret should be filtered")
		}
	}

	// Verify the filtered lines have <removed> placeholder
	for _, line := range cfg.Lines {
		if strings.Contains(line, "enable password") && !strings.Contains(line, "<removed>") {
			t.Errorf("Level 2: filtered enable password should contain <removed>, got: %s", line)
		}
		if strings.Contains(line, "enable secret") && !strings.Contains(line, "<removed>") {
			t.Errorf("Level 2: filtered enable secret should contain <removed>, got: %s", line)
		}
	}
}

// ---------------------------------------------------------------------------
// SNMP community string filtering
// ---------------------------------------------------------------------------

func TestSNMPCommunityFilter(t *testing.T) {
	data := loadTestdata(t, "show_running_config.txt")
	p := &IOSParser{}

	// With NoCommStr=true, community strings should be removed
	cfg, err := p.Parse(data, parse.FilterOpts{NoCommStr: true})
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}

	for _, line := range cfg.Lines {
		if strings.Contains(line, "snmp-server community public") {
			t.Error("SNMP community string 'public' should be filtered")
		}
		if strings.Contains(line, "snmp-server community private") {
			t.Error("SNMP community string 'private' should be filtered")
		}
		if strings.Contains(line, "snmp-server host 10.0.0.50 traps public") {
			t.Error("SNMP host community string should be filtered")
		}
	}

	// Verify <removed> placeholder is present
	commRemoved := false
	hostRemoved := false
	for _, line := range cfg.Lines {
		if strings.Contains(line, "snmp-server community <removed>") {
			commRemoved = true
		}
		if strings.Contains(line, "snmp-server host 10.0.0.50 traps <removed>") {
			hostRemoved = true
		}
	}
	if !commRemoved {
		t.Error("SNMP community line should have <removed> placeholder")
	}
	if !hostRemoved {
		t.Error("SNMP host line should have <removed> placeholder")
	}
}

func TestSNMPCommunityPreserved(t *testing.T) {
	data := loadTestdata(t, "show_running_config.txt")
	p := &IOSParser{}

	// With NoCommStr=false, community strings should be preserved
	cfg, err := p.Parse(data, parse.FilterOpts{NoCommStr: false})
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}

	found := false
	for _, line := range cfg.Lines {
		if strings.Contains(line, "snmp-server community public RO") {
			found = true
		}
	}
	if !found {
		t.Error("SNMP community string should be preserved when NoCommStr=false")
	}
}

// ---------------------------------------------------------------------------
// Blank line collapsing
// ---------------------------------------------------------------------------

func TestBlankBangCollapse(t *testing.T) {
	data := loadTestdata(t, "show_running_config.txt")
	p := &IOSParser{}
	cfg, err := p.Parse(data, parse.FilterOpts{})
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}

	prevBang := false
	for _, line := range cfg.Lines {
		isBang := strings.TrimSpace(line) == "!"
		if isBang && prevBang {
			t.Error("consecutive '!' lines should be collapsed into one")
		}
		prevBang = isBang
	}
}

// ---------------------------------------------------------------------------
// RCS tag neutralization
// ---------------------------------------------------------------------------

func TestRCSTagNeutralization(t *testing.T) {
	data := loadTestdata(t, "show_running_config.txt")
	p := &IOSParser{}
	cfg, err := p.Parse(data, parse.FilterOpts{})
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}

	for _, line := range cfg.Lines {
		if strings.Contains(line, "$Revision:") {
			t.Errorf("RCS $Revision: tag should be neutralized, got: %s", line)
		}
		if strings.Contains(line, "$Id:") {
			t.Errorf("RCS $Id: tag should be neutralized, got: %s", line)
		}
	}

	// Verify the neutralized tags are present
	foundRevision := false
	foundId := false
	for _, line := range cfg.Lines {
		if strings.Contains(line, "Revision: 1.2") && !strings.Contains(line, "$Revision") {
			foundRevision = true
		}
		if strings.Contains(line, "Id: main.cf") && !strings.Contains(line, "$Id") {
			foundId = true
		}
	}
	if !foundRevision {
		t.Error("neutralized Revision tag should be present")
	}
	if !foundId {
		t.Error("neutralized Id tag should be present")
	}
}

// ---------------------------------------------------------------------------
// Combined output (show version + show running-config)
// ---------------------------------------------------------------------------

func TestCombinedOutput(t *testing.T) {
	data := loadTestdata(t, "combined_output.txt")
	p := &IOSParser{}
	cfg, err := p.Parse(data, parse.FilterOpts{FilterPwds: 1, NoCommStr: true})
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}

	// Verify metadata from show version
	if cfg.Metadata["image"] != "C7200-ADVENTERPRISEK9-M" {
		t.Errorf("image = %q, want C7200-ADVENTERPRISEK9-M", cfg.Metadata["image"])
	}
	if cfg.Metadata["serial"] != "FTX1234ABCD" {
		t.Errorf("serial = %q, want FTX1234ABCD", cfg.Metadata["serial"])
	}
	if cfg.Metadata["config_register"] != "0x2102" {
		t.Errorf("config_register = %q, want 0x2102", cfg.Metadata["config_register"])
	}

	// Verify running-config filtering applied
	for _, line := range cfg.Lines {
		if strings.Contains(line, "Building configuration") {
			t.Error("Building configuration header should be removed")
		}
		if strings.Contains(line, "enable password cisco123") {
			t.Error("enable password should be filtered at level 1")
		}
		if strings.Contains(line, "snmp-server community public") {
			t.Error("SNMP community should be filtered when NoCommStr=true")
		}
	}
}

// ---------------------------------------------------------------------------
// DeviceOpts
// ---------------------------------------------------------------------------

func TestDeviceOpts(t *testing.T) {
	p := &IOSParser{}
	opts := p.DeviceOpts()
	if opts.DeviceType != "ios" {
		t.Errorf("DeviceType = %q, want ios", opts.DeviceType)
	}
	if opts.EnableCmd != "enable" {
		t.Errorf("EnableCmd = %q, want enable", opts.EnableCmd)
	}
	if opts.PromptPattern == "" {
		t.Error("PromptPattern should not be empty")
	}
	if len(opts.SetupCommands) == 0 {
		t.Error("SetupCommands should not be empty")
	}
	if opts.DisablePagingCmd != "terminal length 0" {
		t.Errorf("DisablePagingCmd = %q, want terminal length 0", opts.DisablePagingCmd)
	}
}

// ---------------------------------------------------------------------------
// Edge cases
// ---------------------------------------------------------------------------

func TestEmptyInput(t *testing.T) {
	p := &IOSParser{}
	cfg, err := p.Parse([]byte{}, parse.FilterOpts{})
	if err != nil {
		t.Fatalf("Parse error on empty input: %v", err)
	}
	if len(cfg.Lines) != 0 {
		t.Errorf("expected 0 lines for empty input, got %d", len(cfg.Lines))
	}
}

func TestPasswordFilterReplacedWithPlaceholder(t *testing.T) {
	// Verify that filtered password lines preserve the command keyword
	// and replace the value with <removed>
	input := []byte("show running-config\nenable password mysecret\nend\n")
	p := &IOSParser{}
	cfg, err := p.Parse(input, parse.FilterOpts{FilterPwds: 1})
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}

	found := false
	for _, line := range cfg.Lines {
		if strings.Contains(line, "enable password") {
			if !strings.Contains(line, "<removed>") {
				t.Errorf("filtered line should contain <removed>, got: %s", line)
			}
			if strings.Contains(line, "mysecret") {
				t.Errorf("password value should be removed, got: %s", line)
			}
			found = true
		}
	}
	if !found {
		t.Error("enable password line should be present with <removed> placeholder")
	}
}

func TestPPPPasswordFilter(t *testing.T) {
	input := []byte("show running-config\n ppp pap sent-username remote password 7 0822455D0A16\nend\n")
	p := &IOSParser{}
	cfg, err := p.Parse(input, parse.FilterOpts{FilterPwds: 1})
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}

	for _, line := range cfg.Lines {
		if strings.Contains(line, "ppp") && strings.Contains(line, "password") {
			if !strings.Contains(line, "<removed>") {
				t.Errorf("PPP password should be filtered, got: %s", line)
			}
			if strings.Contains(line, "0822455D0A16") {
				t.Errorf("PPP password value should be removed, got: %s", line)
			}
		}
	}
}

func TestCommandDetectionWithPrompt(t *testing.T) {
	// Verify that command detection works with prompt prefixes
	input := []byte("Router#show running-config\nhostname TestRouter\nend\n")
	p := &IOSParser{}
	cfg, err := p.Parse(input, parse.FilterOpts{})
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}

	found := false
	for _, line := range cfg.Lines {
		if strings.Contains(line, "hostname TestRouter") {
			found = true
		}
	}
	if !found {
		t.Error("hostname line should be present when command has prompt prefix")
	}
}

func TestDetectCommand(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"show version", "show_version"},
		{"show running-config", "write_term"},
		{"write term", "write_term"},
		{"write terminal", "write_term"},
		{"Router#show version", "show_version"},
		{"Router>show running-config", "write_term"},
		{"SW1#show running", "write_term"},
		{"hostname Router", ""},
		{"", ""},
	}
	for _, tc := range tests {
		got := detectCommand(tc.input)
		if got != tc.want {
			t.Errorf("detectCommand(%q) = %q, want %q", tc.input, got, tc.want)
		}
	}
}
