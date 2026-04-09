// Package fortigate implements the Fortinet FortiGate device parser for gorancid.
// It processes raw device output from "get system status" and "show" (full configuration)
// commands, applying RANCID-compatible filtering and metadata extraction.
package fortigate

import (
	"bufio"
	"bytes"
	"regexp"
	"strings"

	"gorancid/pkg/connect"
	"gorancid/pkg/parse"
)

func init() {
	parse.Register("fortigate", &FortiGateParser{})
}

// FortiGateParser implements parse.Parser for Fortinet FortiGate devices.
type FortiGateParser struct{}

// DeviceOpts returns connection parameters for the SSH connector.
func (p *FortiGateParser) DeviceOpts() connect.DeviceOpts {
	return connect.DeviceOpts{
		DeviceType:       "fortigate",
		PromptPattern:    `[\r\n][\w./-]+[#\$]\s*$`,
		SetupCommands:    []string{"config system console", "set output standard", "end"},
		EnableCmd:        "",
		DisablePagingCmd: "config system console\nset output standard\nend",
	}
}

// Parse processes raw FortiGate device output and returns a filtered ParsedConfig.
// The output is expected to contain the results of "get system status" and "show"
// commands, typically separated by command echo/prompt boundaries.
func (p *FortiGateParser) Parse(output []byte, filter parse.FilterOpts) (parse.ParsedConfig, error) {
	md := make(map[string]string)
	scanner := bufio.NewScanner(bytes.NewReader(output))
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	var lines []string
	section := sectionUnknown
	inPrivateKey := false

	for scanner.Scan() {
		line := scanner.Text()

		// Detect command section boundaries
		if isCommandHeader(line, "get system status") {
			section = sectionSystemStatus
			continue
		}
		if isCommandHeader(line, "show") ||
			isCommandHeader(line, "show full-configuration") ||
			isCommandHeader(line, "show full configuration") {
			section = sectionShowConf
			continue
		}

		switch section {
		case sectionSystemStatus:
			processed := processSystemStatusLine(line, md, filter)
			if processed != "" {
				lines = append(lines, processed)
			}
		case sectionShowConf:
			processed := processShowConfLine(line, filter, &inPrivateKey)
			if processed != "" {
				lines = append(lines, processed)
			}
		}
	}

	return parse.ParsedConfig{Lines: lines, Metadata: md}, nil
}

// ---------------------------------------------------------------------------
// Section tracking
// ---------------------------------------------------------------------------

type section int

const (
	sectionUnknown     section = iota
	sectionSystemStatus
	sectionShowConf
)

// ---------------------------------------------------------------------------
// Get System Status handling
// ---------------------------------------------------------------------------

var (
	reFGVersion      = regexp.MustCompile(`^Version:\s+(.+)$`)
	reFGSerial       = regexp.MustCompile(`^Serial-Number:\s+(\S+)$`)
	reFGHostname     = regexp.MustCompile(`^Hostname:\s+(.+)$`)
	reSigDB          = regexp.MustCompile(`^(?:APP-DB|AV AI/ML Model|Botnet DB|Extended DB|IPS-DB|IPS-ETDB|IPS Malicious URL Database|Virus-DB|Proxy-APP-DB|Proxy-IPS-ETDB|industrial-db)`)
	reSystemTime     = regexp.MustCompile(`^system time:`)
	reClusterUptime  = regexp.MustCompile(`^Cluster uptime:`)
	reForticlientSig = regexp.MustCompile(`^FortiClient application signature package:`)
	reFGModel        = regexp.MustCompile(`FortiGate-(\S+)`)
)

func processSystemStatusLine(line string, md map[string]string, filter parse.FilterOpts) string {
	trimmed := strings.TrimSpace(line)

	// Always suppress: system time lines
	if reSystemTime.MatchString(trimmed) {
		return ""
	}
	// Always suppress: Cluster uptime lines
	if reClusterUptime.MatchString(trimmed) {
		return ""
	}
	// Always suppress: FortiClient application signature package lines
	if reForticlientSig.MatchString(trimmed) {
		return ""
	}

	// At FilterOsc >= 2, suppress signature database version lines
	if filter.FilterOsc >= 2 {
		if reSigDB.MatchString(trimmed) {
			return ""
		}
	}

	// Extract metadata
	if m := reFGVersion.FindStringSubmatch(trimmed); m != nil {
		md["version"] = strings.TrimSpace(m[1])
	}
	if m := reFGSerial.FindStringSubmatch(trimmed); m != nil {
		md["serial"] = m[1]
	}
	if m := reFGHostname.FindStringSubmatch(trimmed); m != nil {
		md["hostname"] = strings.TrimSpace(m[1])
	}

	// Model extraction: the FortiGate model often appears in the version line
	// e.g. "Version: FortiGate-100F v7.4.2"
	if strings.Contains(trimmed, "FortiGate-") {
		if m := reFGModel.FindStringSubmatch(trimmed); m != nil {
			md["model"] = m[1]
		}
	}

	// Suppress empty lines in system status output
	if trimmed == "" {
		return ""
	}

	return line
}

// ---------------------------------------------------------------------------
// Show Configuration handling
// ---------------------------------------------------------------------------

var (
	// System time extraction timestamp line (FortiGate echoes this in config)
	reConfSystemTime = regexp.MustCompile(`^!\s*System time:`)
	// Config version line (oscillating)
	reConfFileVer = regexp.MustCompile(`conf_file_ver=`)
	// Password/encryption patterns: FortiGate uses "ENC <base64value>" format
	reEncPassword = regexp.MustCompile(`(?i)^(.*\bENC)\s+\S+(.*)`)
	// Last-login pattern
	reLastLogin = regexp.MustCompile(`^(.*\blast-login)\s+\S+(.*)`)
	// Private key markers (may appear mid-line in FortiGate set private-key "...")
	rePrivateKeyBegin = regexp.MustCompile(`-----BEGIN (?:RSA |EC |DSA )?PRIVATE KEY-----`)
	rePrivateKeyEnd   = regexp.MustCompile(`-----END (?:RSA |EC |DSA )?PRIVATE KEY-----`)
	// md5-key pattern
	reMD5Key = regexp.MustCompile(`^(.*\bmd5-key)\s+\S+(.*)`)
)

func processShowConfLine(line string, filter parse.FilterOpts, inPrivateKey *bool) string {
	trimmed := strings.TrimSpace(line)

	// Remove "!System time:" lines
	if reConfSystemTime.MatchString(trimmed) {
		return ""
	}

	// Remove "conf_file_ver=" lines
	if reConfFileVer.MatchString(trimmed) {
		return ""
	}

	// Private key filtering (FilterOsc > 0): suppress entire key block
	if filter.FilterOsc > 0 {
		// If we're inside a private key block, check for the end marker
		if *inPrivateKey {
			if rePrivateKeyEnd.MatchString(trimmed) {
				*inPrivateKey = false
			}
			return "" // suppress all lines inside the key block
		}

		// Check for the beginning of a private key block
		if rePrivateKeyBegin.MatchString(trimmed) {
			// If the END marker is on the same line, just replace the whole line
			if rePrivateKeyEnd.MatchString(trimmed) {
				return "<removed>"
			}
			*inPrivateKey = true
			return "<removed>"
		}

		// Replace md5-key values
		if m := reMD5Key.FindStringSubmatch(trimmed); m != nil {
			return m[1] + " <removed>" + m[2]
		}

		// Replace last-login values
		if m := reLastLogin.FindStringSubmatch(trimmed); m != nil {
			return m[1] + " <removed>" + m[2]
		}
	}

	// Password/encryption filtering
	// If FilterOsc or FilterPwds > 0: replace enc password values
	if filter.FilterOsc > 0 || filter.FilterPwds > 0 {
		if m := reEncPassword.FindStringSubmatch(trimmed); m != nil {
			return m[1] + " <removed>" + m[2]
		}
	}

	return line
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// isCommandHeader detects command echo lines like "get system status" or "show".
func isCommandHeader(line string, cmd string) bool {
	trimmed := strings.TrimSpace(strings.ToLower(line))
	return trimmed == cmd || trimmed == cmd+"\r"
}