// Package junos implements the RANCID-compatible Juniper JunOS device parser.
//
// It processes the combined output of the commands typically collected from a
// JunOS device and produces a filtered configuration suitable for version
// control, along with metadata extracted from version and chassis output.
package junos

import (
	"bufio"
	"bytes"
	"fmt"
	"regexp"
	"strings"

	"gorancid/pkg/connect"
	"gorancid/pkg/parse"
)

// JunOSParser implements parse.Parser for Juniper JunOS devices.
type JunOSParser struct{}

func init() {
	parse.Register("junos", &JunOSParser{})
}

// DeviceOpts returns connection parameters specific to JunOS devices.
func (p *JunOSParser) DeviceOpts() connect.DeviceOpts {
	return connect.DeviceOpts{
		DeviceType:       "junos",
		PromptPattern:    `(?:^|[\r\n])[\w.-]+@[^\r\n]+>\s*$`,
		SetupCommands:    []string{"set cli screen-length 0", "set cli screen-width 0"},
		EnableCmd:        "",
		DisablePagingCmd: "set cli screen-length 0",
	}
}

// Parse processes the raw output collected from a JunOS device and returns
// a filtered configuration and extracted metadata.
func (p *JunOSParser) Parse(output []byte, filter parse.FilterOpts) (parse.ParsedConfig, error) {
	meta := make(map[string]string)
	var lines []string

	scanner := bufio.NewScanner(bytes.NewReader(output))
	section := sectionNone

	for scanner.Scan() {
		line := scanner.Text()

		// Detect section transitions based on command echoes and section headers.
		switch {
		case isShowVersionHeader(line):
			section = sectionVersion
		case isShowChassisHeader(line):
			section = sectionChassis
		case isShowConfigHeader(line):
			section = sectionConfig
		}

		// --- Global filtering: lines that trigger a retry or are always dropped ---
		if isRetryTrigger(line) {
			return parse.ParsedConfig{}, fmt.Errorf("junos: retry-triggering line: %s", strings.TrimSpace(line))
		}
		if isWarningLine(line) {
			continue
		}

		// --- Prompt marker removal is global (can appear in any section) ---
		line = rePromptMarker.ReplaceAllString(line, "")

		// --- Section-specific processing ---
		switch section {
		case sectionVersion:
			flt, done := processVersionLine(line, meta)
			if done {
				continue
			}
			lines = append(lines, flt)
		case sectionChassis:
			flt := processChassisLine(line, filter)
			if flt == "" {
				continue
			}
			lines = append(lines, flt)
		case sectionConfig:
			flt := processConfigLine(line, filter)
			if flt == "" {
				continue
			}
			lines = append(lines, flt)
		default:
			// Lines before any recognised section are kept as-is (command echoes etc.)
			if strings.TrimSpace(line) != "" {
				lines = append(lines, line)
			}
		}
	}

	if len(meta) == 0 {
		meta["junos_info"] = "" // signal that parsing ran but no version was found
	}

	return parse.ParsedConfig{Lines: lines, Metadata: meta}, scanner.Err()
}

// ---------------------------------------------------------------------------
// Section detection helpers
// ---------------------------------------------------------------------------

type section int

const (
	sectionNone section = iota
	sectionVersion
	sectionChassis
	sectionConfig
)

var (
	reShowVersion = regexp.MustCompile(`(?i)^show\s+version(\s+detail)?\s*$`)
	reShowChassis = regexp.MustCompile(`(?i)^show\s+chassis\s+hardware(\s+detail)?\s*$`)
	reShowConfig  = regexp.MustCompile(`(?i)^show\s+configuration(\s*\|\s*no-more)?\s*$`)
)

func isShowVersionHeader(line string) bool {
	return reShowVersion.MatchString(strings.TrimSpace(line))
}

func isShowChassisHeader(line string) bool {
	return reShowChassis.MatchString(strings.TrimSpace(line))
}

func isShowConfigHeader(line string) bool {
	return reShowConfig.MatchString(strings.TrimSpace(line))
}

// ---------------------------------------------------------------------------
// Global line filters
// ---------------------------------------------------------------------------

var (
	reWarning = regexp.MustCompile(`(?i)^warning:`)
	reRetry1  = regexp.MustCompile(`(?i)error:\s*could not connect`)
	reRetry2  = regexp.MustCompile(`Resource deadlock avoided`)
)

func isWarningLine(line string) bool {
	return reWarning.MatchString(line)
}

func isRetryTrigger(line string) bool {
	return reRetry1.MatchString(line) || reRetry2.MatchString(line)
}

// ---------------------------------------------------------------------------
// show version detail processing
// ---------------------------------------------------------------------------

var (
	reJunosInfo = regexp.MustCompile(`^Juniper Networks is:\s+(.+)$`)
	reModel     = regexp.MustCompile(`^Model:\s+(.+)$`)
)

func processVersionLine(line string, meta map[string]string) (string, bool) {
	t := strings.TrimSpace(line)

	if m := reJunosInfo.FindStringSubmatch(t); m != nil {
		meta["junos_info"] = strings.TrimSpace(m[1])
		return "", true
	}

	if m := reModel.FindStringSubmatch(t); m != nil {
		// Keep only the first Model: line encountered.
		if _, exists := meta["model"]; !exists {
			meta["model"] = strings.TrimSpace(m[1])
		}
		return "", true
	}

	return line, false
}

// ---------------------------------------------------------------------------
// show chassis hardware detail processing
// ---------------------------------------------------------------------------

// Items that oscillate on each collection run and should be removed when
// FilterOsc >= 2 (RANCID FILTER_OSC level 2 drops temperatures, uptime, etc.).
var (
	reChassisTemp    = regexp.MustCompile(`(?i)temperature`)
	reChassisUptime  = regexp.MustCompile(`(?i)up\s+\d+\s+(days?|hours?|minutes?|seconds?)`)
	reChassisTimeStr = regexp.MustCompile(`(?i)^\s*(Current time|System uptime|Uptime)`)
)

func processChassisLine(line string, filter parse.FilterOpts) string {
	// Always drop blank chassis lines
	t := strings.TrimSpace(line)
	if t == "" {
		return ""
	}

	// Filter oscillating values when FilterOsc >= 2
	if filter.FilterOsc >= 2 {
		if reChassisTemp.MatchString(t) || reChassisUptime.MatchString(t) || reChassisTimeStr.MatchString(t) {
			return ""
		}
	}

	return line
}

// ---------------------------------------------------------------------------
// show configuration | no-more processing
// ---------------------------------------------------------------------------

// Prompt markers that JunOS appends to line ends: {master}, {backup}, {linecard}, etc.
var rePromptMarker = regexp.MustCompile(`\s*\{(master|backup|linecard|primary|secondary|routing-engine \d+)\}\s*$`)

// Last-commit timestamp comment
var reLastCommit = regexp.MustCompile(`^\s*## last commit:.*$`)

// SECRET-DATA tags appended to lines or as standalone markers
var reSecretDataSuffix = regexp.MustCompile(`\s*##?\s*SECRET-DATA\s*$`)
var reSecretDataInline = regexp.MustCompile(`\s*#\s*SECRET-DATA\b`)

// ---------------------------------------------------------------------------
// Password / secret filtering patterns
// ---------------------------------------------------------------------------

var (
	// Level 1: clear-text secrets
	reAuthKey         = regexp.MustCompile(`^(.*authentication-key\s+)\S+(.*)`)
	reMD5Key          = regexp.MustCompile(`^(.*md5\s+)\S+(\s+key\s+)\S+(.*)`)
	reMD5KeySimple    = regexp.MustCompile(`^(.*md5\s+)\S+(.*)`)
	reHelloAuthKey    = regexp.MustCompile(`^(.*hello-authentication-key\s+)\S+(.*)`)
	rePreSharedKey    = regexp.MustCompile(`^(.*pre-shared-key\s+)(ascii-text|hexadecimal)\s+\S+(.*)`)
	reKeyAsciiHex     = regexp.MustCompile(`^(.*key\s+)(ascii-text|hexadecimal)\s+\S+(.*)`)
	reSecretSimplePwd = regexp.MustCompile(`^(.*(?:secret|simple-password))\s+\S+(.*)`)

	// Level 2: encrypted secrets
	reEncryptedPassword = regexp.MustCompile(`^(.*encrypted-password)\s+\S+(.*)`)
	reSSHRSAKey         = regexp.MustCompile(`^(ssh-rsa\s+"[^"]*"\s+).*$`)
	reSSHDSAKey         = regexp.MustCompile(`^(ssh-dsa\s+"[^"]*"\s+).*$`)
)

// ---------------------------------------------------------------------------
// SNMP community string filtering
// ---------------------------------------------------------------------------

var (
	reCommunityName = regexp.MustCompile(`^(.*community\s+)\S+(.*)`)
	reTrapGroupName = regexp.MustCompile(`^(.*trap-group\s+\S+.*targets\s+)\S+(.*)`)
)

// ---------------------------------------------------------------------------
// License scale / expiry filtering
// ---------------------------------------------------------------------------

var (
	reLicenseScale  = regexp.MustCompile(`^(.*(?:fib|rib|lsp|bgp)\s+scale\s+)\d+(.*)`)
	reLicenseExpiry = regexp.MustCompile(`license\s+expiry\s+\S+\s+\S+`)
)

// processConfigLine applies all configuration filtering rules to a single line.
func processConfigLine(line string, filter parse.FilterOpts) string {
	// Remove last-commit timestamp comment lines
	if reLastCommit.MatchString(line) {
		return ""
	}

	// Remove SECRET-DATA suffixes and inline markers
	line = reSecretDataSuffix.ReplaceAllString(line, "")
	line = reSecretDataInline.ReplaceAllString(line, "")

	// Blank after stripping?
	t := strings.TrimSpace(line)
	if t == "" {
		return ""
	}

	// --- Password filtering ---
	line = filterPasswords(line, filter.FilterPwds)

	// --- SNMP community string filtering ---
	if filter.NoCommStr {
		line = filterCommunity(line)
	}

	// --- License scale filtering ---
	line = filterLicenseScale(line)

	// --- License expiry filtering ---
	line = filterLicenseExpiry(line)

	return line
}

// filterPasswords applies RANCID-style password filtering at the given level.
//
//	Level 0: no filtering
//	Level 1: filter clear-text passwords (authentication-key, md5 key, hello-authentication-key,
//	          pre-shared-key, key ascii-text/hexadecimal, secret/simple-password)
//	Level 2: additionally filter encrypted-password, ssh-rsa, ssh-dsa
func filterPasswords(line string, level int) string {
	if level < 1 {
		return line
	}

	// --- Level 1: clear-text secrets ---
	// authentication-key <value>
	if m := reAuthKey.FindStringSubmatch(line); m != nil {
		return m[1] + "<removed>" + m[2]
	}
	// md5 <hash> key <key>
	if m := reMD5Key.FindStringSubmatch(line); m != nil {
		return m[1] + "<removed>" + m[2] + "<removed>" + m[3]
	}
	// md5 <hash> (without "key" following)
	if m := reMD5KeySimple.FindStringSubmatch(line); m != nil {
		return m[1] + "<removed>" + m[2]
	}
	// hello-authentication-key <value>
	if m := reHelloAuthKey.FindStringSubmatch(line); m != nil {
		return m[1] + "<removed>" + m[2]
	}
	// (pre-shared-)key (ascii-text|hexadecimal) <value>
	if m := rePreSharedKey.FindStringSubmatch(line); m != nil {
		return m[1] + m[2] + " <removed>" + m[3]
	}
	// key (ascii-text|hexadecimal) <value>
	if m := reKeyAsciiHex.FindStringSubmatch(line); m != nil {
		return m[1] + m[2] + " <removed>" + m[3]
	}
	// secret/simple-password <value>
	if m := reSecretSimplePwd.FindStringSubmatch(line); m != nil {
		return m[1] + " <removed>" + m[2]
	}

	// --- Level 2: encrypted / key secrets ---
	if level >= 2 {
		// encrypted-password <value>
		if m := reEncryptedPassword.FindStringSubmatch(line); m != nil {
			return m[1] + " <removed>" + m[2]
		}
		// ssh-rsa "description" <key-data>
		if m := reSSHRSAKey.FindStringSubmatch(line); m != nil {
			return m[1] + " <removed>"
		}
		// ssh-dsa "description" <key-data>
		if m := reSSHDSAKey.FindStringSubmatch(line); m != nil {
			return m[1] + " <removed>"
		}
	}

	return line
}

// filterCommunity replaces SNMP community strings with <removed>.
func filterCommunity(line string) string {
	if m := reCommunityName.FindStringSubmatch(line); m != nil {
		return m[1] + "<removed>" + m[2]
	}
	if m := reTrapGroupName.FindStringSubmatch(line); m != nil {
		return m[1] + "<removed>" + m[2]
	}
	return line
}

// filterLicenseScale replaces FIB/RIB/LSP/BGP scale numbers with <removed>.
func filterLicenseScale(line string) string {
	if m := reLicenseScale.FindStringSubmatch(line); m != nil {
		return m[1] + "<removed>" + m[2]
	}
	return line
}

// filterLicenseExpiry replaces non-permanent license expiry values with <limited>.
func filterLicenseExpiry(line string) string {
	if reLicenseExpiry.MatchString(line) {
		// Replace the date value with <limited>
		return reLicenseExpiry.ReplaceAllString(line, "license expiry <limited>")
	}
	return line
}
