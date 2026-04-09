package junos

import (
	"regexp"
	"strings"
	"testing"

	"gorancid/pkg/parse"
)

// ---------------------------------------------------------------------------
// Test data — realistic JunOS output snippets
// ---------------------------------------------------------------------------

const testShowVersion = `show version detail
Hostname: core-rtr-01
Model: mx960
Juniper Networks is: JUNOS 21.4R3-S2.3
JUNOS Base OS Software Suite [21.4R3-S2.3]
JUNOS Platform Support Suite [21.4R3-S2.3]
warning: some warning text
Model: MX960
`

const testShowChassisHardware = `show chassis hardware detail
Hardware inventory:
Item             Version  Part number  Serial number     Description
Chassis                                JW1234567890      MX960
Midplane         REV 07   711-031511   AB9876543210      MX960 Midplane
FPC 0            REV 08   750-060823   CD1122334455      MPC3D-16XGE-SFPP
  CPU            REV 04   711-030613   EF5566778899      AMPC PM
    PIC 0                BUILTIN      BUILTIN            4x 10GE SFP+
Temperature: 38 degrees C
Uptime: 12 days, 3 hours, 45 minutes
Current time: 2026-04-09 14:30:00 UTC
`

const testShowConfiguration = `show configuration | no-more
## last commit: "2026-04-09 10:15:00 UTC by admin"
version 21.4R3-S2.3;
groups {
    global {
        system {
            name-server {
                10.0.0.1;
                10.0.0.2;
            }
        }
    }
}
system {
    host-name core-rtr-01;
    root-authentication {
        encrypted-password "$6$xyz123$abcdef"; ## SECRET-DATA
    }
    login {
        user admin {
            uid 1000;
            class super-user;
            authentication {
                encrypted-password "$6$abc456$ghijkl"; ## SECRET-DATA
                ssh-rsa "admin-key" ssh-rsa AAAAB3NzaC1yc2EAAAADAQABAAABgQC7...;
            }
        }
        user ops {
            uid 2000;
            class operator;
            authentication {
                simple-password mysecretpass # SECRET-DATA
            }
        }
    }
    services {
        ssh {
            protocol-version v2;
        }
        snmp {
            community public {
                authorization read-only;
            }
            community private {
                authorization read-write;
            }
            trap-group monitoring {
                targets {
                    10.0.0.100;
                }
            }
        }
    }
    ntp {
        server 10.0.0.5;
    }
}
protocols {
    bgp {
        group internal {
            type internal;
            authentication-key "$9$abc123def456"; ## SECRET-DATA
            neighbor 10.0.1.1;
        }
        group external {
            type external;
            md5 $1$abcd key $9$key123;
            neighbor 172.16.0.1;
        }
    }
    ospf {
        area 0.0.0.0 {
            interface ge-0/0/0.0 {
                hello-authentication-key "$9$ospf123";
            }
        }
    }
    isis {
        interface all.0 {
            hello-authentication-key "$9$isis456";
        }
    }
}
security {
    ike {
        proposal ike-prop {
            authentication-method pre-shared-key;
            pre-shared-key ascii-text "$9$ike-secret-key"; ## SECRET-DATA
        }
        policy ike-pol {
            proposals ike-prop;
            pre-shared-key hexadecimal 0123456789abcdef; ## SECRET-DATA
        }
    }
    ipsec {
        proposal ipsec-prop {
            protocol esp;
        }
    }
    pki {
        ca-profile root-ca {
            url https://ca.example.com;
        }
    }
}
chassis {
    fpc 0 {
        pic 0 {
            tunnel-services {
                bandwidth 10g;
            }
        }
    }
}
license {
    capacity {
        fib scale 2000000;
        rib scale 3000000;
        bgp scale 5000;
        lsp scale 10000;
    }
    expiry non-permanent 2026-12-31;
}
secret myadminsecret # SECRET-DATA
{master}
`

// Full combined output simulating the complete device dump.
const testFullOutput = testShowVersion + "\n" + testShowChassisHardware + "\n" + testShowConfiguration

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func defaultFilter() parse.FilterOpts {
	return parse.FilterOpts{
		FilterPwds: 1,
		FilterOsc:  2,
		NoCommStr:  true,
	}
}

func parseOutput(t *testing.T, input string, filter parse.FilterOpts) parse.ParsedConfig {
	t.Helper()
	p := &JunOSParser{}
	cfg, err := p.Parse([]byte(input), filter)
	if err != nil {
		t.Fatalf("Parse returned unexpected error: %v", err)
	}
	return cfg
}

// ---------------------------------------------------------------------------
// Registration test
// ---------------------------------------------------------------------------

func TestRegistered(t *testing.T) {
	_, ok := parse.Lookup("junos")
	if !ok {
		t.Fatal("junos parser not registered")
	}
}

// ---------------------------------------------------------------------------
// Version extraction
// ---------------------------------------------------------------------------

func TestVersionExtraction(t *testing.T) {
	cfg := parseOutput(t, testShowVersion, defaultFilter())

	if got := cfg.Metadata["junos_info"]; got != "JUNOS 21.4R3-S2.3" {
		t.Errorf("junos_info = %q, want %q", got, "JUNOS 21.4R3-S2.3")
	}
	if got := cfg.Metadata["model"]; got != "mx960" {
		t.Errorf("model = %q, want %q", got, "mx960")
	}
}

// ---------------------------------------------------------------------------
// Warning filtering
// ---------------------------------------------------------------------------

func TestWarningFiltered(t *testing.T) {
	cfg := parseOutput(t, testShowVersion, defaultFilter())
	for _, l := range cfg.Lines {
		if strings.Contains(l, "warning:") {
			t.Errorf("warning line not filtered: %q", l)
		}
	}
}

// ---------------------------------------------------------------------------
// Retry-trigger detection
// ---------------------------------------------------------------------------

func TestRetryTrigger(t *testing.T) {
	p := &JunOSParser{}
	cases := []string{
		"error: could not connect to 10.0.0.1",
		"Resource deadlock avoided",
	}
	for _, tc := range cases {
		_, err := p.Parse([]byte(tc), defaultFilter())
		if err == nil {
			t.Errorf("expected retry error for %q, got nil", tc)
		}
	}
}

// ---------------------------------------------------------------------------
// Chassis hardware — oscillating value filtering
// ---------------------------------------------------------------------------

func TestChassisOscFiltering(t *testing.T) {
	cfg := parseOutput(t, testShowChassisHardware, defaultFilter())
	for _, l := range cfg.Lines {
		if strings.Contains(l, "Temperature:") {
			t.Errorf("temperature line not filtered: %q", l)
		}
		if strings.Contains(l, "Uptime:") {
			t.Errorf("uptime line not filtered: %q", l)
		}
		if strings.Contains(l, "Current time:") {
			t.Errorf("current time line not filtered: %q", l)
		}
	}
	// Hardware lines must survive
	found := false
	for _, l := range cfg.Lines {
		if strings.Contains(l, "JW1234567890") {
			found = true
		}
	}
	if !found {
		t.Error("expected chassis serial line to survive filtering")
	}
}

func TestChassisOscOff(t *testing.T) {
	filter := defaultFilter()
	filter.FilterOsc = 0
	cfg := parseOutput(t, testShowChassisHardware, filter)
	found := false
	for _, l := range cfg.Lines {
		if strings.Contains(l, "Temperature:") {
			found = true
		}
	}
	if !found {
		t.Error("temperature line should survive when FilterOsc=0")
	}
}

// ---------------------------------------------------------------------------
// Prompt marker removal
// ---------------------------------------------------------------------------

func TestPromptMarkerRemoval(t *testing.T) {
	cfg := parseOutput(t, "some line {master}\nother line\n", defaultFilter())
	for _, l := range cfg.Lines {
		if strings.Contains(l, "{master}") {
			t.Errorf("prompt marker not removed: %q", l)
		}
	}
}

func TestPromptMarkerBackup(t *testing.T) {
	input := "interface ge-0/0/0 {backup}\n"
	cfg := parseOutput(t, input, defaultFilter())
	for _, l := range cfg.Lines {
		if strings.Contains(l, "{backup}") {
			t.Errorf("backup marker not removed: %q", l)
		}
	}
}

func TestPromptMarkerLinecard(t *testing.T) {
	input := "interface ge-0/0/0 {linecard}\n"
	cfg := parseOutput(t, input, defaultFilter())
	for _, l := range cfg.Lines {
		if strings.Contains(l, "{linecard}") {
			t.Errorf("linecard marker not removed: %q", l)
		}
	}
}

// ---------------------------------------------------------------------------
// Last-commit timestamp removal
// ---------------------------------------------------------------------------

func TestLastCommitRemoved(t *testing.T) {
	cfg := parseOutput(t, testShowConfiguration, defaultFilter())
	for _, l := range cfg.Lines {
		if strings.Contains(l, "last commit:") {
			t.Errorf("last commit line not removed: %q", l)
		}
	}
}

// ---------------------------------------------------------------------------
// SECRET-DATA removal
// ---------------------------------------------------------------------------

func TestSecretDataRemoved(t *testing.T) {
	cfg := parseOutput(t, testShowConfiguration, defaultFilter())
	for _, l := range cfg.Lines {
		if strings.Contains(l, "SECRET-DATA") {
			t.Errorf("SECRET-DATA not removed: %q", l)
		}
	}
}

// ---------------------------------------------------------------------------
// Password filtering — level 1
// ---------------------------------------------------------------------------

func TestPasswordFilterLevel1AuthKey(t *testing.T) {
	filter := defaultFilter()
	filter.FilterPwds = 1
	input := "show configuration | no-more\nauthentication-key \"$9$abc123def\";\n"
	cfg := parseOutput(t, input, filter)
	found := false
	for _, l := range cfg.Lines {
		if strings.Contains(l, "authentication-key") {
			if strings.Contains(l, "$9$abc123def") {
				t.Errorf("clear-text authentication-key not filtered: %q", l)
			}
			if !strings.Contains(l, "<removed>") {
				t.Errorf("authentication-key value not replaced with <removed>: %q", l)
			}
			found = true
		}
	}
	if !found {
		t.Error("authentication-key line missing from output")
	}
}

func TestPasswordFilterLevel1HelloAuthKey(t *testing.T) {
	filter := defaultFilter()
	filter.FilterPwds = 1
	input := "show configuration | no-more\nhello-authentication-key \"$9$ospf123\";\n"
	cfg := parseOutput(t, input, filter)
	for _, l := range cfg.Lines {
		if strings.Contains(l, "hello-authentication-key") && strings.Contains(l, "$9$ospf123") {
			t.Errorf("hello-authentication-key not filtered: %q", l)
		}
	}
}

func TestPasswordFilterLevel1PreSharedKeyAscii(t *testing.T) {
	filter := defaultFilter()
	filter.FilterPwds = 1
	input := "show configuration | no-more\npre-shared-key ascii-text \"$9$ike-secret-key\";\n"
	cfg := parseOutput(t, input, filter)
	for _, l := range cfg.Lines {
		if strings.Contains(l, "pre-shared-key") && strings.Contains(l, "$9$ike-secret-key") {
			t.Errorf("pre-shared-key ascii-text not filtered: %q", l)
		}
	}
}

func TestPasswordFilterLevel1PreSharedKeyHex(t *testing.T) {
	filter := defaultFilter()
	filter.FilterPwds = 1
	input := "show configuration | no-more\npre-shared-key hexadecimal 0123456789abcdef;\n"
	cfg := parseOutput(t, input, filter)
	for _, l := range cfg.Lines {
		if strings.Contains(l, "pre-shared-key") && strings.Contains(l, "0123456789abcdef") {
			t.Errorf("pre-shared-key hexadecimal not filtered: %q", l)
		}
	}
}

func TestPasswordFilterLevel1KeyAscii(t *testing.T) {
	filter := defaultFilter()
	filter.FilterPwds = 1
	input := "show configuration | no-more\nkey ascii-text secretvalue;\n"
	cfg := parseOutput(t, input, filter)
	for _, l := range cfg.Lines {
		if strings.Contains(l, "key ascii-text") && strings.Contains(l, "secretvalue") {
			t.Errorf("key ascii-text not filtered: %q", l)
		}
	}
}

func TestPasswordFilterLevel1SecretValue(t *testing.T) {
	filter := defaultFilter()
	filter.FilterPwds = 1
	input := "show configuration | no-more\nsecret myadminsecret;\n"
	cfg := parseOutput(t, input, filter)
	for _, l := range cfg.Lines {
		if strings.Contains(l, "secret") && strings.Contains(l, "myadminsecret") {
			t.Errorf("secret value not filtered: %q", l)
		}
	}
}

func TestPasswordFilterLevel1SimplePassword(t *testing.T) {
	filter := defaultFilter()
	filter.FilterPwds = 1
	input := "show configuration | no-more\nsimple-password hunter2;\n"
	cfg := parseOutput(t, input, filter)
	for _, l := range cfg.Lines {
		if strings.Contains(l, "simple-password") && strings.Contains(l, "hunter2") {
			t.Errorf("simple-password not filtered: %q", l)
		}
	}
}

func TestPasswordFilterLevel1MD5Key(t *testing.T) {
	filter := defaultFilter()
	filter.FilterPwds = 1
	input := "show configuration | no-more\nmd5 $1$abcd key $9$key123;\n"
	cfg := parseOutput(t, input, filter)
	for _, l := range cfg.Lines {
		if strings.Contains(l, "md5") && strings.Contains(l, "$9$key123") {
			t.Errorf("md5 key not filtered: %q", l)
		}
	}
}

// ---------------------------------------------------------------------------
// Password filtering — level 2 (encrypted-password, ssh keys)
// ---------------------------------------------------------------------------

func TestPasswordFilterLevel2EncryptedPassword(t *testing.T) {
	filter := defaultFilter()
	filter.FilterPwds = 2
	input := "show configuration | no-more\nencrypted-password \"$6$xyz123$abcdef\";\n"
	cfg := parseOutput(t, input, filter)
	for _, l := range cfg.Lines {
		if strings.Contains(l, "encrypted-password") && strings.Contains(l, "$6$xyz123$abcdef") {
			t.Errorf("encrypted-password not filtered at level 2: %q", l)
		}
	}
}

func TestPasswordFilterLevel1KeepsEncrypted(t *testing.T) {
	filter := defaultFilter()
	filter.FilterPwds = 1
	input := "show configuration | no-more\nencrypted-password \"$6$xyz123$abcdef\";\n"
	cfg := parseOutput(t, input, filter)
	found := false
	for _, l := range cfg.Lines {
		if strings.Contains(l, "encrypted-password") {
			if strings.Contains(l, "<removed>") {
				t.Errorf("encrypted-password should NOT be filtered at level 1: %q", l)
			}
			found = true
		}
	}
	if !found {
		t.Error("encrypted-password line missing from output at level 1")
	}
}

func TestPasswordFilterLevel2SSHRSA(t *testing.T) {
	filter := defaultFilter()
	filter.FilterPwds = 2
	input := "show configuration | no-more\nssh-rsa \"admin-key\" ssh-rsa AAAAB3NzaC1yc2EAAAADAQABAAABgQC7abcdef;\n"
	cfg := parseOutput(t, input, filter)
	for _, l := range cfg.Lines {
		if strings.Contains(l, "ssh-rsa") && strings.Contains(l, "AAAAB3NzaC1yc2E") {
			t.Errorf("ssh-rsa key not filtered at level 2: %q", l)
		}
	}
}

func TestPasswordFilterLevel2SSHDSA(t *testing.T) {
	filter := defaultFilter()
	filter.FilterPwds = 2
	input := "show configuration | no-more\nssh-dsa \"ops-key\" ssh-dsa AAAAB3NzaC1kc3MAAACBAJ;\n"
	cfg := parseOutput(t, input, filter)
	for _, l := range cfg.Lines {
		if strings.Contains(l, "ssh-dsa") && strings.Contains(l, "AAAAB3NzaC1kc3MAAACBAJ") {
			t.Errorf("ssh-dsa key not filtered at level 2: %q", l)
		}
	}
}

// ---------------------------------------------------------------------------
// Password filtering — level 0 (no filtering)
// ---------------------------------------------------------------------------

func TestPasswordFilterLevel0(t *testing.T) {
	filter := defaultFilter()
	filter.FilterPwds = 0
	input := "show configuration | no-more\nauthentication-key \"$9$abc123def\";\nencrypted-password \"$6$xyz\";\n"
	cfg := parseOutput(t, input, filter)
	for _, l := range cfg.Lines {
		if strings.Contains(l, "authentication-key") && strings.Contains(l, "<removed>") {
			t.Errorf("level 0 should not filter authentication-key: %q", l)
		}
		if strings.Contains(l, "encrypted-password") && strings.Contains(l, "<removed>") {
			t.Errorf("level 0 should not filter encrypted-password: %q", l)
		}
	}
}

// ---------------------------------------------------------------------------
// SNMP community string filtering
// ---------------------------------------------------------------------------

func TestCommunityStringFiltering(t *testing.T) {
	filter := defaultFilter()
	filter.NoCommStr = true
	input := "show configuration | no-more\ncommunity public {\ncommunity private {\n"
	cfg := parseOutput(t, input, filter)
	for _, l := range cfg.Lines {
		if strings.Contains(l, "community public") {
			t.Errorf("SNMP community string 'public' not filtered: %q", l)
		}
		if strings.Contains(l, "community private") {
			t.Errorf("SNMP community string 'private' not filtered: %q", l)
		}
	}
}

func TestCommunityStringOff(t *testing.T) {
	filter := defaultFilter()
	filter.NoCommStr = false
	input := "show configuration | no-more\ncommunity public {\n"
	cfg := parseOutput(t, input, filter)
	found := false
	for _, l := range cfg.Lines {
		if strings.Contains(l, "community public") {
			found = true
		}
	}
	if !found {
		t.Error("community string should survive when NoCommStr=false")
	}
}

// ---------------------------------------------------------------------------
// License scale filtering
// ---------------------------------------------------------------------------

func TestLicenseScaleFiltering(t *testing.T) {
	filter := defaultFilter()
	input := "show configuration | no-more\nfib scale 2000000;\nrib scale 3000000;\nbgp scale 5000;\nlsp scale 10000;\n"
	cfg := parseOutput(t, input, filter)
	for _, l := range cfg.Lines {
		if strings.Contains(l, "scale") {
			if strings.Contains(l, "2000000") || strings.Contains(l, "3000000") || strings.Contains(l, "5000") || strings.Contains(l, "10000") {
				t.Errorf("license scale number not filtered: %q", l)
			}
		}
	}
}

// ---------------------------------------------------------------------------
// License expiry filtering
// ---------------------------------------------------------------------------

func TestLicenseExpiryFiltering(t *testing.T) {
	filter := defaultFilter()
	input := "show configuration | no-more\nlicense expiry non-permanent 2026-12-31;\n"
	cfg := parseOutput(t, input, filter)
	for _, l := range cfg.Lines {
		if strings.Contains(l, "expiry") && strings.Contains(l, "2026-12-31") {
			t.Errorf("license expiry date not replaced with <limited>: %q", l)
		}
	}
}

// ---------------------------------------------------------------------------
// Full integration test
// ---------------------------------------------------------------------------

func TestFullParse(t *testing.T) {
	cfg := parseOutput(t, testFullOutput, defaultFilter())

	// Metadata
	if cfg.Metadata["junos_info"] != "JUNOS 21.4R3-S2.3" {
		t.Errorf("junos_info = %q, want %q", cfg.Metadata["junos_info"], "JUNOS 21.4R3-S2.3")
	}
	if cfg.Metadata["model"] != "mx960" {
		t.Errorf("model = %q, want %q", cfg.Metadata["model"], "mx960")
	}

	// No warning lines
	for _, l := range cfg.Lines {
		if strings.Contains(l, "warning:") {
			t.Errorf("warning line in output: %q", l)
		}
	}

	// No SECRET-DATA
	for _, l := range cfg.Lines {
		if strings.Contains(l, "SECRET-DATA") {
			t.Errorf("SECRET-DATA in output: %q", l)
		}
	}

	// No last-commit
	for _, l := range cfg.Lines {
		if strings.Contains(l, "last commit:") {
			t.Errorf("last commit in output: %q", l)
		}
	}

	// No {master} markers
	for _, l := range cfg.Lines {
		if strings.Contains(l, "{master}") {
			t.Errorf("{master} marker in output: %q", l)
		}
	}

	// SNMP community strings replaced
	for _, l := range cfg.Lines {
		if strings.Contains(l, "community public") || strings.Contains(l, "community private") {
			t.Errorf("SNMP community string in output: %q", l)
		}
	}

	// License scale numbers replaced
	for _, l := range cfg.Lines {
		if strings.Contains(l, "scale") && (strings.Contains(l, "2000000") || strings.Contains(l, "3000000")) {
			t.Errorf("license scale number in output: %q", l)
		}
	}
}

// ---------------------------------------------------------------------------
// Unit tests for individual helper functions
// ---------------------------------------------------------------------------

func TestFilterPasswordsAuthKey(t *testing.T) {
	result := filterPasswords(`authentication-key "$9$abc";`, 1)
	if !strings.Contains(result, "<removed>") || strings.Contains(result, "$9$abc") {
		t.Errorf("auth key not filtered: %q", result)
	}
}

func TestFilterPasswordsMD5Simple(t *testing.T) {
	result := filterPasswords(`md5 $1$abcd;`, 1)
	if !strings.Contains(result, "<removed>") || strings.Contains(result, "$1$abcd") {
		t.Errorf("md5 simple not filtered: %q", result)
	}
}

func TestFilterPasswordsSecret(t *testing.T) {
	result := filterPasswords(`secret s3cret;`, 1)
	if !strings.Contains(result, "<removed>") || strings.Contains(result, "s3cret") {
		t.Errorf("secret not filtered: %q", result)
	}
}

func TestFilterPasswordsSimplePassword(t *testing.T) {
	result := filterPasswords(`simple-password p@ss;`, 1)
	if !strings.Contains(result, "<removed>") || strings.Contains(result, "p@ss") {
		t.Errorf("simple-password not filtered: %q", result)
	}
}

func TestFilterPasswordsEncryptedLevel1(t *testing.T) {
	result := filterPasswords(`encrypted-password "$6$xyz";`, 1)
	if strings.Contains(result, "<removed>") {
		t.Errorf("encrypted-password should not be filtered at level 1: %q", result)
	}
}

func TestFilterPasswordsEncryptedLevel2(t *testing.T) {
	result := filterPasswords(`encrypted-password "$6$xyz";`, 2)
	if !strings.Contains(result, "<removed>") || strings.Contains(result, "$6$xyz") {
		t.Errorf("encrypted-password not filtered at level 2: %q", result)
	}
}

func TestFilterPasswordsLevel0(t *testing.T) {
	result := filterPasswords(`authentication-key "$9$abc";`, 0)
	if strings.Contains(result, "<removed>") {
		t.Errorf("level 0 should not filter: %q", result)
	}
}

func TestFilterCommunity(t *testing.T) {
	result := filterCommunity(`community public {`)
	if strings.Contains(result, "public") || !strings.Contains(result, "<removed>") {
		t.Errorf("community name not filtered: %q", result)
	}
}

func TestFilterCommunityNoMatch(t *testing.T) {
	result := filterCommunity(`interface ge-0/0/0 {`)
	if result != `interface ge-0/0/0 {` {
		t.Errorf("non-community line should not be modified: %q", result)
	}
}

func TestFilterLicenseScale(t *testing.T) {
	result := filterLicenseScale(`fib scale 2000000;`)
	if strings.Contains(result, "2000000") || !strings.Contains(result, "<removed>") {
		t.Errorf("fib scale not filtered: %q", result)
	}
}

func TestFilterLicenseExpiry(t *testing.T) {
	result := filterLicenseExpiry(`license expiry non-permanent 2026-12-31;`)
	if strings.Contains(result, "2026-12-31") || !strings.Contains(result, "<limited>") {
		t.Errorf("license expiry not replaced: %q", result)
	}
}

// ---------------------------------------------------------------------------
// Regex correctness spot checks
// ---------------------------------------------------------------------------

func TestPromptMarkerRegex(t *testing.T) {
	cases := []struct {
		input    string
		expected string
	}{
		{"line {master}", "line"},
		{"line {backup}", "line"},
		{"line {linecard}", "line"},
		{"line {primary}", "line"},
		{"line {secondary}", "line"},
		{"line {routing-engine 0}", "line"},
		{"no marker here", "no marker here"},
	}
	for _, tc := range cases {
		result := rePromptMarker.ReplaceAllString(tc.input, "")
		if strings.TrimSpace(result) != strings.TrimSpace(tc.expected) {
			t.Errorf("prompt marker regex: input=%q got=%q want=%q", tc.input, result, tc.expected)
		}
	}
}

// ---------------------------------------------------------------------------
// DeviceOpts
// ---------------------------------------------------------------------------

func TestDeviceOpts(t *testing.T) {
	p := &JunOSParser{}
	opts := p.DeviceOpts()
	if opts.DeviceType != "junos" {
		t.Errorf("DeviceType = %q, want %q", opts.DeviceType, "junos")
	}
	if opts.EnableCmd != "" {
		t.Errorf("EnableCmd = %q, want empty", opts.EnableCmd)
	}
	if opts.DisablePagingCmd != "set cli screen-length 0" {
		t.Errorf("DisablePagingCmd = %q, want %q", opts.DisablePagingCmd, "set cli screen-length 0")
	}
	if len(opts.SetupCommands) != 2 {
		t.Fatalf("SetupCommands len = %d, want 2", len(opts.SetupCommands))
	}
	if opts.SetupCommands[0] != "set cli screen-length 0" {
		t.Errorf("SetupCommands[0] = %q, want %q", opts.SetupCommands[0], "set cli screen-length 0")
	}
	if opts.SetupCommands[1] != "set cli screen-width 0" {
		t.Errorf("SetupCommands[1] = %q, want %q", opts.SetupCommands[1], "set cli screen-width 0")
	}
	// Validate prompt pattern compiles
	if _, err := regexp.Compile(opts.PromptPattern); err != nil {
		t.Errorf("PromptPattern does not compile: %v", err)
	}
}