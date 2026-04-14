// Package ios implements the Cisco IOS device parser for gorancid.
// It processes raw device output from "show version" and "show running-config"
// commands, applying RANCID-compatible filtering and metadata extraction.
package ios

import (
	"bufio"
	"bytes"
	"regexp"
	"strings"

	"gorancid/pkg/connect"
	"gorancid/pkg/parse"
)

func init() {
	parse.Register("ios", &IOSParser{})
	parse.RegisterAlias("cisco", "ios")
	parse.RegisterAlias("cat5k", "ios")
}

// IOSParser implements parse.Parser for Cisco IOS devices.
type IOSParser struct{}

// DeviceOpts returns connection parameters for the SSH connector.
func (p *IOSParser) DeviceOpts() connect.DeviceOpts {
	return connect.DeviceOpts{
		DeviceType:       "ios",
		PromptPattern:    `(?:^|[\r\n])[\w./:-]+(?:\([^)]+\))*[#>]\s*$`,
		SetupCommands:    []string{"terminal length 0", "terminal width 0"},
		EnableCmd:        "enable",
		DisablePagingCmd: "terminal length 0",
	}
}

// Parse processes raw IOS device output and returns a filtered ParsedConfig.
// The output is expected to contain command echoes (e.g. "show version",
// "Router#show running-config") that delimit sections.
func (p *IOSParser) Parse(output []byte, filter parse.FilterOpts) (parse.ParsedConfig, error) {
	md := make(map[string]string)
	scanner := bufio.NewScanner(bytes.NewReader(output))
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	var lines []string
	inShowVersion := false
	inWriteTerm := false
	prevWasBang := false

	for scanner.Scan() {
		line := scanner.Text()

		// Detect command boundaries. Command echoes may have a prompt prefix
		// like "Router#" or "Router>" that we strip before matching.
		cmd := detectCommand(line)
		if cmd == "show_version" {
			inShowVersion = true
			inWriteTerm = false
			prevWasBang = false
			continue
		}
		if cmd == "write_term" {
			inShowVersion = false
			inWriteTerm = true
			prevWasBang = false
			continue
		}

		if inShowVersion {
			processed := processShowVersionLine(line, md)
			if processed != "" {
				lines = append(lines, processed)
			}
			continue
		}

		if inWriteTerm {
			processed := processWriteTermLine(line, md, filter, &prevWasBang)
			if processed != "" {
				lines = append(lines, processed)
			}
			continue
		}
	}

	return parse.ParsedConfig{Lines: lines, Metadata: md}, nil
}

// ---------------------------------------------------------------------------
// Command detection
// ---------------------------------------------------------------------------

var rePromptPrefix = regexp.MustCompile(`^[\w./-]+[#>]\s*`)

// detectCommand checks whether a line is a command echo (with or without
// a prompt prefix) and returns the detected command type.
func detectCommand(line string) string {
	trimmed := strings.TrimSpace(strings.ToLower(line))
	// Strip optional prompt prefix like "Router#" or "Router>"
	trimmed = rePromptPrefix.ReplaceAllString(trimmed, "")
	trimmed = strings.TrimSpace(trimmed)
	// Strip trailing \r
	trimmed = strings.TrimRight(trimmed, "\r")

	switch trimmed {
	case "show version":
		return "show_version"
	case "show running-config", "show running", "write term", "write terminal":
		return "write_term"
	default:
		return ""
	}
}

// ---------------------------------------------------------------------------
// Show Version handling
// ---------------------------------------------------------------------------

var (
	reVersionImage = regexp.MustCompile(`Cisco IOS .* Software,? \(([A-Za-z0-9_-]+)\), .*Version\s+(.+)`)
	reChassisProc  = regexp.MustCompile(`(?:Cisco\s+)?(\S+(?:\sseries)?)\s+\(([^)]+)\)\s+\(revision[^)]+\)\s+processor\s+.*\bwith\s+(\d+[kK])`)
	reSerial       = regexp.MustCompile(`(?i)processor board id (\S+)`)
	reConfigReg    = regexp.MustCompile(`Configuration register is (.+)`)
	reBootImage    = regexp.MustCompile(`System image file is "([^"]*)"`)
	rePager        = regexp.MustCompile(`<[-]+ More [-]+>`)
	reLoadFive     = regexp.MustCompile(`^Load for five`)
	reTimeSource   = regexp.MustCompile(`^Time source is`)
)

func processShowVersionLine(line string, md map[string]string) string {
	// Filter pager output
	if rePager.MatchString(line) {
		return ""
	}
	// Filter "Load for five" lines
	if reLoadFive.MatchString(line) {
		return ""
	}
	// Filter "Time source is" lines
	if reTimeSource.MatchString(line) {
		return ""
	}
	// Filter empty lines
	if strings.TrimSpace(line) == "" {
		return ""
	}

	// Extract metadata
	if m := reVersionImage.FindStringSubmatch(line); m != nil {
		md["image"] = m[1]
		md["version"] = strings.TrimSpace(m[2])
	}
	if m := reChassisProc.FindStringSubmatch(line); m != nil {
		md["processor"] = m[1]
		if m[2] != "" {
			md["cpu"] = m[2]
		}
		md["memory"] = strings.ToLower(m[3])
	}
	if m := reSerial.FindStringSubmatch(line); m != nil {
		md["serial"] = m[1]
	}
	if m := reConfigReg.FindStringSubmatch(line); m != nil {
		md["config_register"] = m[1]
	}
	if m := reBootImage.FindStringSubmatch(line); m != nil {
		md["boot_image"] = m[1]
	}

	return line
}

// ---------------------------------------------------------------------------
// Write Term (running-config) handling
// ---------------------------------------------------------------------------

var (
	reBuildingConfig   = regexp.MustCompile(`^Building configuration\.\.\.`)
	reCurrentConfig    = regexp.MustCompile(`^Current configuration :`)
	reLastConfigChange = regexp.MustCompile(`^! Last configuration change`)
	reWrittenBy        = regexp.MustCompile(`^: Written by`)
	reSaved            = regexp.MustCompile(`^: Saved`)
	reNoConfigChange   = regexp.MustCompile(`^! no configuration change since last restart`)
	reNtpClockPeriod   = regexp.MustCompile(`^\s*ntp clock-period`)
	reTftpServerFlash  = regexp.MustCompile(`^\s*tftp-server flash`)
	reClockrate        = regexp.MustCompile(`^\s+clockrate`)
	reFairQueue        = regexp.MustCompile(`^\s+fair-queue individual-limit`)
	reRCSTag           = regexp.MustCompile(`\$Revision:`)
	reIdTag            = regexp.MustCompile(`\$Id:`)
)

// Password and secret patterns for filtering.
// Each regex captures the keyword portion in group 1 and the trailing text
// in group 2, so the password value (the word between them) can be replaced
// with "<removed>".
var (
	// Level 1: clear-text passwords (also filtered at level 2)
	reEnablePassword      = regexp.MustCompile(`^(\s*enable password)\s+\S+(.*)`)
	reUsernamePassword    = regexp.MustCompile(`^(\s*username\s+\S+\s+password)\s+\S+(.*)`)
	reOspfAuthKey         = regexp.MustCompile(`^(\s*ip ospf authentication-key)\s+\S+(.*)`)
	reOspfMD5Key          = regexp.MustCompile(`^(\s*ip ospf message-digest-key)\s+\S+\s+md5\s+\S+(.*)`)
	reIsisPassword        = regexp.MustCompile(`^(\s*(?:isis|domain|area)[\s-]password)\s+\S+(.*)`)
	reIsakmpKey           = regexp.MustCompile(`^(\s*crypto isakmp key)\s+\S+(.*)`)
	rePreSharedKey        = regexp.MustCompile(`^(\s*pre-shared-key(?:\s+\S+)?)\s+\S+(.*)`)
	reKeyString           = regexp.MustCompile(`^(\s*key-string)\s+\S+(.*)`)
	reFailoverKey         = regexp.MustCompile(`^(\s*failover key)\s+\S+(.*)`)
	reHsrpAuth            = regexp.MustCompile(`^(\s*standby\s+\S+\s+authentication)\s+\S+(.*)`)
	reBgpNeighborPassword = regexp.MustCompile(`^(\s*neighbor\s+\S+\s+password)\s+\S+(.*)`)
	reCableSharedSecret   = regexp.MustCompile(`^(\s*cable shared-secret)\s+\S+(.*)`)
	rePppPassword         = regexp.MustCompile(`^(\s*ppp\s+(?:chap|pap)\s+\S+(?:\s+\S+)*\s+password)\s+\S+\s+\S+`)
	reFtpPassword         = regexp.MustCompile(`^(\s*ip ftp password)\s+\S+(.*)`)
	reLinePassword        = regexp.MustCompile(`^(\s*password)\s+\S+(.*)`)

	// Level 2: also filter secrets/encrypted passwords
	reEnableSecret   = regexp.MustCompile(`^(\s*enable secret)\s+\S+(.*)`)
	reUsernameSecret = regexp.MustCompile(`^(\s*username\s+\S+\s+secret)\s+\S+(.*)`)
	reLineSecret     = regexp.MustCompile(`^(\s*secret)\s+\S+(.*)`)
)

// SNMP community string patterns
var (
	reSnmpCommunity = regexp.MustCompile(`^(\s*snmp-server community)\s+\S+(.*)`)
	reSnmpHost      = regexp.MustCompile(`^(\s*snmp-server host\s+\S+(?:\s+(?:traps|informs))?)\s+\S+(.*)`)
)

func processWriteTermLine(line string, md map[string]string, filter parse.FilterOpts, prevWasBang *bool) string {
	// Remove "Building configuration..." header
	if reBuildingConfig.MatchString(line) {
		return ""
	}
	// Remove "Current configuration :" line
	if reCurrentConfig.MatchString(line) {
		return ""
	}
	// Remove timestamp lines
	if reLastConfigChange.MatchString(line) {
		return ""
	}
	if reWrittenBy.MatchString(line) {
		return ""
	}
	if reSaved.MatchString(line) {
		return ""
	}
	// Remove "no configuration change" line
	if reNoConfigChange.MatchString(line) {
		return ""
	}
	// Remove ntp clock-period (oscillating)
	if reNtpClockPeriod.MatchString(line) {
		return ""
	}
	// Remove tftp-server flash lines
	if reTftpServerFlash.MatchString(line) {
		return ""
	}
	// Remove clockrate on serial interfaces
	if reClockrate.MatchString(line) {
		return ""
	}
	// Remove fair-queue individual-limit
	if reFairQueue.MatchString(line) {
		return ""
	}

	// Neutralize RCS/CVS tags
	if reRCSTag.MatchString(line) {
		line = strings.ReplaceAll(line, "$Revision:", "Revision:")
	}
	if reIdTag.MatchString(line) {
		line = strings.ReplaceAll(line, "$Id:", "Id:")
	}

	// Collapse consecutive blank "!" lines into a single "!"
	if strings.TrimSpace(line) == "!" {
		if *prevWasBang {
			return ""
		}
		*prevWasBang = true
		return line
	}
	*prevWasBang = false

	// Filter SNMP community strings
	if filter.NoCommStr {
		if m := reSnmpCommunity.FindStringSubmatch(line); m != nil {
			if len(m) >= 3 {
				return m[1] + " <removed>" + m[2]
			}
			return m[1] + " <removed>"
		}
		if m := reSnmpHost.FindStringSubmatch(line); m != nil {
			if len(m) >= 3 {
				return m[1] + " <removed>" + m[2]
			}
			return m[1] + " <removed>"
		}
	}

	// Filter passwords based on FilterPwds level
	line = filterPasswords(line, filter.FilterPwds)

	return line
}

// filterPasswords applies password filtering based on the level.
// Level 0: no filtering
// Level 1: filter clear-text passwords
// Level 2: also filter secrets and encrypted passwords
func filterPasswords(line string, level int) string {
	if level == 0 {
		return line
	}

	// Level 1 patterns (also applied at level 2).
	// Order matters: more specific patterns are checked first.
	level1Patterns := []*regexp.Regexp{
		reUsernamePassword,
		reEnablePassword,
		reOspfMD5Key,
		reOspfAuthKey,
		reIsisPassword,
		reIsakmpKey,
		rePreSharedKey,
		reKeyString,
		reFailoverKey,
		reHsrpAuth,
		reBgpNeighborPassword,
		reCableSharedSecret,
		rePppPassword,
		reFtpPassword,
		reLinePassword,
	}

	for _, pat := range level1Patterns {
		if m := pat.FindStringSubmatch(line); m != nil {
			if len(m) >= 3 {
				return m[1] + " <removed>" + m[2]
			}
			return m[1] + " <removed>"
		}
	}

	// Level 2: also filter secrets and encrypted passwords
	if level >= 2 {
		level2Patterns := []*regexp.Regexp{
			reUsernameSecret,
			reEnableSecret,
			reLineSecret,
		}
		for _, pat := range level2Patterns {
			if m := pat.FindStringSubmatch(line); m != nil {
				if len(m) >= 3 {
					return m[1] + " <removed>" + m[2]
				}
				return m[1] + " <removed>"
			}
		}
	}

	return line
}
