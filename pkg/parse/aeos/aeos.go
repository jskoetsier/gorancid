// Package aeos implements the Arista EOS device parser for gorancid.
// It processes raw device output from "show version", "show boot-config",
// "show env all", "show inventory", "show boot-extensions", "show extensions",
// "diff startup-config running-config", and "show running-config" commands,
// applying RANCID-compatible filtering and metadata extraction.
package aeos

import (
	"bufio"
	"bytes"
	"regexp"
	"strings"

	"gorancid/pkg/connect"
	"gorancid/pkg/parse"
)

func init() {
	parse.Register("aeos", &EOSParser{})
}

// EOSParser implements parse.Parser for Arista EOS devices.
type EOSParser struct{}

// DeviceOpts returns connection parameters for the SSH connector.
func (p *EOSParser) DeviceOpts() connect.DeviceOpts {
	return connect.DeviceOpts{
		DeviceType:       "aeos",
		PromptPattern:    `(?:\[\d{1,2}:\d{2}\]\s*)?[\w./-]+(?:\([^)]+\))*[>#]\s*$`,
		SetupCommands:    []string{"terminal length 0", "terminal width 0"},
		EnableCmd:        "",
		DisablePagingCmd: "terminal length 0",
	}
}

// ---------------------------------------------------------------------------
// Section tracking
// ---------------------------------------------------------------------------

type section int

const (
	sectionUnknown section = iota
	sectionShowVersion
	sectionShowBoot
	sectionShowEnv
	sectionShowInventory
	sectionShowBootExt
	sectionShowExt
	sectionDiffConfig
	sectionWriteTerm
)

// ---------------------------------------------------------------------------
// Parse
// ---------------------------------------------------------------------------

// Parse processes raw Arista EOS device output and returns a filtered ParsedConfig.
func (p *EOSParser) Parse(output []byte, filter parse.FilterOpts) (parse.ParsedConfig, error) {
	md := make(map[string]string)
	scanner := bufio.NewScanner(bytes.NewReader(output))
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	var lines []string
	section := sectionUnknown
	prevWasBang := false
	showExtHeaderEmitted := false
	modelEmitted := false
	var diffLines []string

	for scanner.Scan() {
		line := scanner.Text()

		// Detect command section boundaries
		if isCommandHeader(line, "show version") {
			section = sectionShowVersion
			continue
		}
		if isCommandHeader(line, "show boot-config") {
			section = sectionShowBoot
			continue
		}
		if isCommandHeader(line, "show env all") {
			section = sectionShowEnv
			continue
		}
		if isCommandHeader(line, "show inventory") {
			section = sectionShowInventory
			continue
		}
		if isCommandHeader(line, "show boot-extensions") {
			section = sectionShowBootExt
			continue
		}
		if isCommandHeader(line, "show extensions") {
			section = sectionShowExt
			showExtHeaderEmitted = false
			continue
		}
		if isCommandHeader(line, "diff startup-config running-config") {
			section = sectionDiffConfig
			continue
		}
		if isCommandHeader(line, "show running-config") {
			section = sectionWriteTerm
			prevWasBang = false
			continue
		}

		switch section {
		case sectionShowVersion:
			processed := processShowVersionLine(line, md, &modelEmitted)
			if processed != "" {
				lines = append(lines, processed)
			}
		case sectionShowBoot:
			processed := processShowBootLine(line)
			if processed != "" {
				lines = append(lines, processed)
			}
		case sectionShowEnv:
			processed := processShowEnvLine(line)
			if processed != "" {
				lines = append(lines, processed)
			}
		case sectionShowInventory:
			processed := processShowInventoryLine(line)
			if processed != "" {
				lines = append(lines, processed)
			}
		case sectionShowBootExt:
			processed := processShowBootExtLine(line)
			if processed != "" {
				lines = append(lines, processed)
			}
		case sectionShowExt:
			processed := processShowExtLine(line, &showExtHeaderEmitted)
			if processed != "" {
				lines = append(lines, processed)
			}
		case sectionDiffConfig:
			trimmed := strings.TrimSpace(line)
			if trimmed != "" {
				diffLines = append(diffLines, trimmed)
			}
		case sectionWriteTerm:
			processed := processWriteTermLine(line, filter, &prevWasBang)
			if processed != "" {
				lines = append(lines, processed)
			}
		}
	}

	// If diff had content, emit unsaved changes banner
	if len(diffLines) > 0 {
		lines = append(lines, "!******************************")
		lines = append(lines, "!*** unsaved changes exist ***")
		lines = append(lines, "!******************************")
	}

	return parse.ParsedConfig{Lines: lines, Metadata: md}, nil
}

// ---------------------------------------------------------------------------
// Command header detection
// ---------------------------------------------------------------------------

var reEOSPromptPrefix = regexp.MustCompile(`^(?:\[\d{1,2}:\d{2}\]\s*)?[\w./-]+(?:\([^)]+\))*[#>]\s*`)

// isCommandHeader detects command echo lines, stripping the EOS prompt prefix.
func isCommandHeader(line string, cmd string) bool {
	trimmed := strings.TrimSpace(strings.ToLower(line))
	trimmed = reEOSPromptPrefix.ReplaceAllString(trimmed, "")
	trimmed = strings.TrimSpace(trimmed)
	trimmed = strings.TrimRight(trimmed, "\r")
	return trimmed == cmd
}

// ---------------------------------------------------------------------------
// Show Version handling
// ---------------------------------------------------------------------------

var (
	reEOSModelLine  = regexp.MustCompile(`^Arista\s+(\S+)`)
	reEOSKeyValue   = regexp.MustCompile(`^([A-Za-z][A-Za-z 0-9]*?):\s+(.+)$`)
	reEOSUptime     = regexp.MustCompile(`(?i)^Uptime:`)
	reEOSFreeMemory = regexp.MustCompile(`(?i)^Free memory:`)
)

func processShowVersionLine(line string, md map[string]string, modelEmitted *bool) string {
	trimmed := strings.TrimSpace(line)
	if trimmed == "" {
		return ""
	}

	// Skip oscillating values
	if reEOSUptime.MatchString(trimmed) {
		return ""
	}
	if reEOSFreeMemory.MatchString(trimmed) {
		return ""
	}

	// First non-blank line is the model
	if !*modelEmitted {
		*modelEmitted = true
		if m := reEOSModelLine.FindStringSubmatch(trimmed); m != nil {
			md["model"] = m[1]
		}
		return "!Model: " + trimmed
	}

	// Key: Value lines
	if m := reEOSKeyValue.FindStringSubmatch(trimmed); m != nil {
		key := strings.TrimSpace(m[1])
		value := strings.TrimSpace(m[2])
		switch key {
		case "Serial number":
			md["serial"] = value
		case "Software image version":
			md["version"] = value
		}
		return "!" + key + ": " + value
	}

	return "!" + trimmed
}

// ---------------------------------------------------------------------------
// Show Boot handling
// ---------------------------------------------------------------------------

func processShowBootLine(line string) string {
	trimmed := strings.TrimSpace(line)
	if trimmed == "" {
		return ""
	}
	return "!" + trimmed
}

// ---------------------------------------------------------------------------
// Show Env handling
// ---------------------------------------------------------------------------

func processShowEnvLine(line string) string {
	trimmed := strings.TrimSpace(line)
	if trimmed == "" {
		return ""
	}
	return "!" + trimmed
}

// ---------------------------------------------------------------------------
// Show Inventory handling
// ---------------------------------------------------------------------------

func processShowInventoryLine(line string) string {
	trimmed := strings.TrimSpace(line)
	if trimmed == "" {
		return ""
	}
	return "!" + trimmed
}

// ---------------------------------------------------------------------------
// Show Boot Extensions handling
// ---------------------------------------------------------------------------

func processShowBootExtLine(line string) string {
	trimmed := strings.TrimSpace(line)
	if trimmed == "" {
		return ""
	}
	return "!BootExtension: " + trimmed
}

// ---------------------------------------------------------------------------
// Show Extensions handling
// ---------------------------------------------------------------------------

func processShowExtLine(line string, headerEmitted *bool) string {
	trimmed := strings.TrimSpace(line)
	if trimmed == "" {
		return ""
	}
	if !*headerEmitted {
		*headerEmitted = true
		return "!Extensions:\n!" + trimmed
	}
	return "!" + trimmed
}

// ---------------------------------------------------------------------------
// Diff startup-config running-config handling
// ---------------------------------------------------------------------------

// (Handled inline in Parse via diffLines accumulation)

// ---------------------------------------------------------------------------
// Show Running-Config handling (WriteTerm)
// ---------------------------------------------------------------------------

var (
	reEOSCommandHeader = regexp.MustCompile(`^! Command: show running-config`)
	reEOSDeviceHeader  = regexp.MustCompile(`^! device:`)
	reEOSTimeHeader    = regexp.MustCompile(`^! Time:`)
	// SNMP
	reEOSSnmpCommunity = regexp.MustCompile(`^(\s*snmp-server community)\s+\S+(.*)`)
	reEOSSnmpHost      = regexp.MustCompile(`^(\s*snmp-server host\s+\S+(?:\s+(?:traps|informs))?)\s+\S+(.*)`)
	// Passwords / secrets
	reEOSSecret = regexp.MustCompile(`^(\s*username\s+\S+.*\bsecret)\s+\S+(.*)`)
	reEOSNtpKey = regexp.MustCompile(`^(\s*ntp (?:authentication-key|trusted-key))\s+\S+(.*)`)
	// BGP neighbor passwords
	reEOSBgpPassword = regexp.MustCompile(`^(\s*neighbor\s+\S+\s+password)\s+\S+(.*)`)
	// OSPF authentication
	reEOSOspfKey     = regexp.MustCompile(`^(\s*ip ospf authentication-key)\s+\S+(.*)`)
	reEOSOspfMsgKey  = regexp.MustCompile(`^(\s*ip ospf message-digest-key)\s+\S+(.*)`)
	reEOSIsisKey     = regexp.MustCompile(`^(\s*isis authentication-key)\s+\S+(.*)`)
	reEOSIsisMd5Key  = regexp.MustCompile(`^(\s*isis authentication-type md5)\s+\S+(.*)`)
	// RCS tags
	reEOSRCSTag = regexp.MustCompile(`\$((Revision|Id):)[^$]*\$`)
)

func processWriteTermLine(line string, filter parse.FilterOpts, prevWasBang *bool) string {
	trimmed := strings.TrimSpace(line)

	// Remove command headers
	if reEOSCommandHeader.MatchString(trimmed) {
		return ""
	}
	if reEOSDeviceHeader.MatchString(trimmed) {
		return ""
	}
	if reEOSTimeHeader.MatchString(trimmed) {
		return ""
	}

	// End marker
	if trimmed == "end" {
		return "end"
	}

	// Collapse consecutive "!" lines
	if trimmed == "!" {
		if *prevWasBang {
			return ""
		}
		*prevWasBang = true
		return "!"
	}
	*prevWasBang = false

	// Blank lines
	if trimmed == "" {
		return ""
	}

	// Neutralize RCS tags
	if reEOSRCSTag.MatchString(line) {
		line = reEOSRCSTag.ReplaceAllString(line, "$$$1$$")
	}

	// SNMP community string filtering
	if filter.NoCommStr {
		if m := reEOSSnmpCommunity.FindStringSubmatch(line); m != nil {
			return m[1] + " <removed>" + m[2]
		}
		if m := reEOSSnmpHost.FindStringSubmatch(line); m != nil {
			return m[1] + " <removed>" + m[2]
		}
	}

	// Password / secret filtering (FilterPwds >= 1)
	if filter.FilterPwds >= 1 {
		if m := reEOSSecret.FindStringSubmatch(line); m != nil {
			return m[1] + " <removed>" + m[2]
		}
		if m := reEOSNtpKey.FindStringSubmatch(line); m != nil {
			return m[1] + " <removed>" + m[2]
		}
		if m := reEOSBgpPassword.FindStringSubmatch(line); m != nil {
			return m[1] + " <removed>" + m[2]
		}
		if m := reEOSOspfKey.FindStringSubmatch(line); m != nil {
			return m[1] + " <removed>" + m[2]
		}
		if m := reEOSOspfMsgKey.FindStringSubmatch(line); m != nil {
			return m[1] + " <removed>" + m[2]
		}
		if m := reEOSIsisKey.FindStringSubmatch(line); m != nil {
			return m[1] + " <removed>" + m[2]
		}
	}

	return line
}