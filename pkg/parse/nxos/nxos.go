package nxos

import (
	"bufio"
	"bytes"
	"fmt"
	"regexp"
	"strings"

	"gorancid/pkg/connect"
	"gorancid/pkg/parse"
)

func init() {
	parse.Register("nxos", &NXOSParser{})
}

// NXOSParser implements parse.Parser for Cisco NX-OS devices.
type NXOSParser struct{}

// DeviceOpts returns connection parameters for the SSH connector.
func (p *NXOSParser) DeviceOpts() connect.DeviceOpts {
	return connect.DeviceOpts{
		DeviceType:       "nxos",
		PromptPattern:    `(?:^|[\r\n])[\w./:-]+(?:\([^)]+\))*#\s*$`,
		SetupCommands:    []string{"terminal length 0", "terminal width 0"},
		EnableCmd:        "",
		DisablePagingCmd: "terminal length 0",
	}
}

// Parse processes raw NX-OS device output and returns a filtered ParsedConfig.
func (p *NXOSParser) Parse(output []byte, filter parse.FilterOpts) (parse.ParsedConfig, error) {
	md := make(map[string]string)
	scanner := bufio.NewScanner(bytes.NewReader(output))
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	var lines []string
	inShowVersion := false
	inWriteTerm := false
	prevWasBang := false

	for scanner.Scan() {
		line := scanner.Text()

		// Detect command boundaries
		switch detectCommand(line) {
		case "show_version":
			inShowVersion = true
			inWriteTerm = false
			prevWasBang = false
			continue
		case "write_term":
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

var rePromptPrefix = regexp.MustCompile(`^[\w./-]+#\s*`)

func detectCommand(line string) string {
	trimmed := strings.TrimSpace(strings.ToLower(line))
	trimmed = rePromptPrefix.ReplaceAllString(trimmed, "")
	trimmed = strings.TrimSpace(trimmed)
	trimmed = strings.TrimRight(trimmed, "\r")

	switch trimmed {
	case "show version":
		return "show_version"
	case "show running-config", "show running":
		return "write_term"
	default:
		return ""
	}
}

// ---------------------------------------------------------------------------
// Show Version handling
// ---------------------------------------------------------------------------

var (
	reNXOSVersion   = regexp.MustCompile(`(?:Cisco )?Nexus\s+(\S+)\s+.*Version\s+.*\((\S+)\)`)
	reNXOSChassis   = regexp.MustCompile(`(?:cisco\s+)(Nexus[\d]+)\s+.*[Cc]hassis`)
	reNXOSSoftware  = regexp.MustCompile(`\s+kickstart:\s+version\s+(\S+)`)
	reNXOSMemory    = regexp.MustCompile(`(\d+)\s+kB`)
	reNXOSBootImage = regexp.MustCompile(`System image file is:\s+(\S+)`)
	reNXOSInvalid   = regexp.MustCompile(`^(?:Invalid input detected|Type help|% Invalid command|No token match|% Permission denied|command authorization failed)`)
	reNXOSType      = regexp.MustCompile(`Cisco (Nexus|Storage Area Networking) Operating System`)
)

func processShowVersionLine(line string, md map[string]string) string {
	// Skip empty lines
	if strings.TrimSpace(line) == "" {
		return ""
	}
	// Skip error lines
	if reNXOSInvalid.MatchString(line) {
		return ""
	}

	// Detect device type
	if reNXOSType.MatchString(line) {
		md["type"] = "NXOS"
	}

	// Extract chassis model
	if m := reNXOSChassis.FindStringSubmatch(line); m != nil {
		md["processor"] = m[1]
	}

	// Extract version
	if m := reNXOSSoftware.FindStringSubmatch(line); m != nil {
		md["version"] = m[1]
	}

	// Extract boot image
	if m := reNXOSBootImage.FindStringSubmatch(line); m != nil {
		md["boot_image"] = m[1]
	}

	// Convert kB to MB for memory
	if m := reNXOSMemory.FindStringSubmatch(line); m != nil {
		kb := 0
		fmt.Sscanf(m[1], "%d", &kb)
		if kb > 0 {
			mb := kb / 1024
			line = reNXOSMemory.ReplaceAllString(line, fmt.Sprintf("%d MB", mb))
		}
	}

	return line
}

// ---------------------------------------------------------------------------
// Write Term (running-config) handling
// ---------------------------------------------------------------------------

var (
	reNXOSCommandHeader  = regexp.MustCompile(`^!Command: show running-config`)
	reNXOSTimeHeader     = regexp.MustCompile(`^!Time:`)
	reNXOSBuilding       = regexp.MustCompile(`^Building configuration\.\.\.`)
	reNXOSCurrentConfig  = regexp.MustCompile(`^Current configuration`)
	reNXOSLastConfig     = regexp.MustCompile(`^! Last configuration change`)
	reNXOSNoConfigChange = regexp.MustCompile(`^! no configuration change since last restart`)
	reNXOSNtpClock       = regexp.MustCompile(`^\s*ntp clock-period`)
	reNXOSTftpFlash      = regexp.MustCompile(`^\s*tftp-server flash`)
	reNXOSFairQueue      = regexp.MustCompile(`^\s+fair-queue individual-limit`)
	reNXOSClockrate      = regexp.MustCompile(`^\s+clockrate`)
	reNXOSRCSTag         = regexp.MustCompile(`\$Revision:`)
	reNXOSIdTag          = regexp.MustCompile(`\$Id:`)
	reNXOSEnd            = regexp.MustCompile(`^end\b`)
)

// Password patterns
var (
	reNXOSEnablePassword   = regexp.MustCompile(`^(\s*enable password)\s+\S+(.*)`)
	reNXOSUsernamePassword = regexp.MustCompile(`^(\s*username\s+\S+\s+password)\s+\S+(.*)`)
	reNXOSNeighborPassword = regexp.MustCompile(`^(\s*neighbor\s+\S+\s+password)\s+\S+(.*)`)
	reNXOSLinePassword     = regexp.MustCompile(`^(\s*password)\s+\S+(.*)`)
	reNXOSOspfKey          = regexp.MustCompile(`^(\s*ip ospf (?:authentication-key|message-digest-key))\s+\S+(.*)`)
	reNXOSIsakmpKey        = regexp.MustCompile(`^(\s*crypto isakmp key)\s+\S+(.*)`)
	reNXOSPreSharedKey     = regexp.MustCompile(`^(\s*pre-shared-key(?:\s+\S+)?)\s+\S+(.*)`)
	reNXOSKeyString        = regexp.MustCompile(`^(\s*key-string)\s+\S+(.*)`)

	reNXOSEnableSecret   = regexp.MustCompile(`^(\s*enable secret)\s+\S+.*`)
	reNXOSUsernameSecret = regexp.MustCompile(`^(\s*username\s+\S+\s+secret)\s+\S+.*`)
	reNXOSLineSecret     = regexp.MustCompile(`^(\s*secret)\s+\S+.*`)

	reNXOSSnmpCommunity = regexp.MustCompile(`^(\s*snmp-server community)\s+\S+(.*)`)
	reNXOSSnmpHost      = regexp.MustCompile(`^(\s*snmp-server host\s+\S+)\s+\S+(.*)`)
	reNXOSTacacsKey     = regexp.MustCompile(`^(\s*(?:tacacs|radius)-server\s+\S+\s+key)\s+\S+(.*)`)
)

func processWriteTermLine(line string, md map[string]string, filter parse.FilterOpts, prevWasBang *bool) string {
	// Remove command header and time stamp
	if reNXOSCommandHeader.MatchString(line) {
		return ""
	}
	if reNXOSTimeHeader.MatchString(line) {
		return ""
	}
	if reNXOSBuilding.MatchString(line) {
		return ""
	}
	if reNXOSCurrentConfig.MatchString(line) {
		return ""
	}
	if reNXOSLastConfig.MatchString(line) {
		return ""
	}
	if reNXOSNoConfigChange.MatchString(line) {
		return ""
	}
	if reNXOSNtpClock.MatchString(line) {
		return ""
	}
	if reNXOSTftpFlash.MatchString(line) {
		return ""
	}
	if reNXOSFairQueue.MatchString(line) {
		return ""
	}
	if reNXOSClockrate.MatchString(line) {
		return ""
	}

	// Neutralize RCS tags
	if reNXOSRCSTag.MatchString(line) {
		line = strings.ReplaceAll(line, "$Revision:", "Revision:")
	}
	if reNXOSIdTag.MatchString(line) {
		line = strings.ReplaceAll(line, "$Id:", "Id:")
	}

	// Collapse consecutive blank "!" lines
	if strings.TrimSpace(line) == "!" {
		if *prevWasBang {
			return ""
		}
		*prevWasBang = true
	} else {
		*prevWasBang = false
	}

	// End marker
	if reNXOSEnd.MatchString(line) {
		return line
	}

	// SNMP community filtering
	if filter.NoCommStr {
		if m := reNXOSSnmpCommunity.FindStringSubmatch(line); m != nil {
			return m[1] + " <removed>" + m[2]
		}
		if m := reNXOSSnmpHost.FindStringSubmatch(line); m != nil {
			return m[1] + " <removed>" + m[2]
		}
	}

	// TACACS/RADIUS key filtering
	if filter.FilterPwds >= 1 {
		if m := reNXOSTacacsKey.FindStringSubmatch(line); m != nil {
			return m[1] + " <removed>" + m[2]
		}
	}

	// Password filtering
	line = filterNXOSPasswords(line, filter.FilterPwds)

	return line
}

func filterNXOSPasswords(line string, level int) string {
	if level == 0 {
		return line
	}

	level1Patterns := []*regexp.Regexp{
		reNXOSUsernamePassword,
		reNXOSEnablePassword,
		reNXOSNeighborPassword,
		reNXOSOspfKey,
		reNXOSIsakmpKey,
		reNXOSPreSharedKey,
		reNXOSKeyString,
		reNXOSLinePassword,
	}

	for _, pat := range level1Patterns {
		if m := pat.FindStringSubmatch(line); m != nil {
			if len(m) >= 3 {
				return m[1] + " <removed>" + m[2]
			}
			return m[1] + " <removed>"
		}
	}

	if level >= 2 {
		level2Patterns := []*regexp.Regexp{
			reNXOSUsernameSecret,
			reNXOSEnableSecret,
			reNXOSLineSecret,
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
