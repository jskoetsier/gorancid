// Package sros implements the Nokia SR OS (TiMOS) device parser for gorancid.
// It handles both Classic CLI (sros) and MD-CLI (sros-md) variants.
// Commands collected: show system information, show chassis, show card state/detail,
// show debug, show bof, admin display-config index, admin display-config.
package sros

import (
	"bufio"
	"bytes"
	"regexp"
	"strings"

	"gorancid/pkg/connect"
	"gorancid/pkg/parse"
)

func init() {
	parse.Register("sros", &SROSParser{})
	parse.RegisterAlias("sros-md", "sros")
	parse.RegisterAlias("nokia", "sros")
	parse.RegisterAlias("timos", "sros")
}

// SROSParser implements parse.Parser for Nokia SR OS devices.
type SROSParser struct{}

// DeviceOpts returns connection parameters for Nokia SR OS.
// Classic CLI prompt: "A:hostname# " or "*A:hostname# " (dirty = asterisk prefix).
func (p *SROSParser) DeviceOpts() connect.DeviceOpts {
	return connect.DeviceOpts{
		DeviceType:       "sros",
		PromptPattern:    `(?:^|[\r\n])\*?[AB]:[^\r\n#>]+[#>]\s*$`,
		SetupCommands:    []string{"environment no more"},
		EnableCmd:        "",
		DisablePagingCmd: "environment no more",
	}
}

// ---------------------------------------------------------------------------
// Section tracking
// ---------------------------------------------------------------------------

type section int

const (
	sectionNone section = iota
	sectionSystemInfo
	sectionChassis
	sectionCardState
	sectionCardDetail
	sectionDebug
	sectionBOF
	sectionConfigIndex
	sectionConfig
)

// ---------------------------------------------------------------------------
// Parse
// ---------------------------------------------------------------------------

func (p *SROSParser) Parse(output []byte, filter parse.FilterOpts) (parse.ParsedConfig, error) {
	md := make(map[string]string)
	var lines []string
	sec := sectionNone

	scanner := bufio.NewScanner(bytes.NewReader(output))
	scanner.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)

	for scanner.Scan() {
		line := scanner.Text()
		line = strings.TrimRight(line, "\r")

		// Section transitions based on command echoes.
		if s, ok := detectSection(line); ok {
			sec = s
			continue
		}

		switch sec {
		case sectionSystemInfo:
			if l := processSystemInfoLine(line, md); l != "" {
				lines = append(lines, l)
			}
		case sectionChassis, sectionCardState, sectionCardDetail:
			if !isVolatileLine(line) && strings.TrimSpace(line) != "" {
				lines = append(lines, line)
			}
		case sectionDebug:
			lines = append(lines, line)
		case sectionBOF:
			if !isBOFVolatile(line) {
				lines = append(lines, line)
			}
		case sectionConfigIndex, sectionConfig:
			if l := processConfigLine(line, filter); l != "" {
				lines = append(lines, l)
			}
		}
	}

	return parse.ParsedConfig{Lines: lines, Metadata: md}, nil
}

// ---------------------------------------------------------------------------
// Command / section detection
// ---------------------------------------------------------------------------

var rePromptPrefix = regexp.MustCompile(`^\*?[AB]:[^\r\n#>]+[#>]\s*`)

var sectionCommands = []struct {
	pattern *regexp.Regexp
	sec     section
}{
	{regexp.MustCompile(`(?i)show\s+system\s+information`), sectionSystemInfo},
	{regexp.MustCompile(`(?i)show\s+chassis\b`), sectionChassis},
	{regexp.MustCompile(`(?i)show\s+card\s+state`), sectionCardState},
	{regexp.MustCompile(`(?i)show\s+card\s+detail`), sectionCardDetail},
	{regexp.MustCompile(`(?i)show\s+debug`), sectionDebug},
	{regexp.MustCompile(`(?i)show\s+bof`), sectionBOF},
	{regexp.MustCompile(`(?i)admin\s+display-config\s+index`), sectionConfigIndex},
	{regexp.MustCompile(`(?i)admin\s+(?:display-config|show\s+configuration)`), sectionConfig},
}

func detectSection(line string) (section, bool) {
	stripped := rePromptPrefix.ReplaceAllString(strings.TrimSpace(line), "")
	stripped = strings.TrimSpace(stripped)
	for _, sc := range sectionCommands {
		if sc.pattern.MatchString(stripped) {
			return sc.sec, true
		}
	}
	return sectionNone, false
}

// ---------------------------------------------------------------------------
// show system information
// ---------------------------------------------------------------------------

var (
	reSysName    = regexp.MustCompile(`System Name\s*:\s*(.+)`)
	reSysType    = regexp.MustCompile(`System Type\s*:\s*(.+)`)
	reSysVersion = regexp.MustCompile(`(?:TiMOS|System Version)\s*:\s*(.+)`)
	reSysSerial  = regexp.MustCompile(`Chassis Serial Number\s*:\s*(\S+)`)
	reUptime     = regexp.MustCompile(`(?i)system up time|uptime`)
	reLastBoot   = regexp.MustCompile(`(?i)last (?:boot|restart)`)
)

func processSystemInfoLine(line string, md map[string]string) string {
	// Volatile: uptime and last-boot lines change every run — drop them.
	if reUptime.MatchString(line) || reLastBoot.MatchString(line) {
		return ""
	}
	if m := reSysName.FindStringSubmatch(line); m != nil {
		md["name"] = strings.TrimSpace(m[1])
	}
	if m := reSysType.FindStringSubmatch(line); m != nil {
		md["model"] = strings.TrimSpace(m[1])
	}
	if m := reSysVersion.FindStringSubmatch(line); m != nil {
		md["version"] = strings.TrimSpace(m[1])
	}
	if m := reSysSerial.FindStringSubmatch(line); m != nil {
		md["serial"] = strings.TrimSpace(m[1])
	}
	if strings.TrimSpace(line) == "" {
		return ""
	}
	return line
}

// ---------------------------------------------------------------------------
// Chassis / card volatile filtering
// ---------------------------------------------------------------------------

var reVolatile = regexp.MustCompile(
	`(?i)(?:temperature|fan speed|actual speed|uptime|up time|last (?:boot|restart)|current time|system time)`,
)

func isVolatileLine(line string) bool {
	return reVolatile.MatchString(line)
}

// ---------------------------------------------------------------------------
// BOF (boot options file)
// ---------------------------------------------------------------------------

var reBOFTimestamp = regexp.MustCompile(`(?i)last modified|modification time|boot count`)

func isBOFVolatile(line string) bool {
	return reBOFTimestamp.MatchString(line)
}

// ---------------------------------------------------------------------------
// admin display-config
// ---------------------------------------------------------------------------

var (
	// Timestamps / generated-by headers inside config output.
	reConfigTimestamp = regexp.MustCompile(`(?i)# generated|# TiMOS|echo ".*generated|# Mon |# Tue |# Wed |# Thu |# Fri |# Sat |# Sun `)
	// Community strings in SNMP context.
	reSnmpCommunity = regexp.MustCompile(`(?i)(community\s+")([^"]+)(")`)
	// Passwords: password, authentication-key, hmac-key values.
	rePassword = regexp.MustCompile(`(?i)((?:password|hmac-md5-key|hmac-sha-key|authentication-key|md5|sha)\s+)("[^"]+"|\S+)`)
)

func processConfigLine(line string, filter parse.FilterOpts) string {
	if reConfigTimestamp.MatchString(line) {
		return ""
	}
	if strings.TrimSpace(line) == "" {
		return ""
	}

	if filter.NoCommStr {
		if reSnmpCommunity.MatchString(line) {
			line = reSnmpCommunity.ReplaceAllString(line, `${1}<removed>${3}`)
		}
	}

	if filter.FilterPwds >= 1 {
		if rePassword.MatchString(line) {
			line = rePassword.ReplaceAllString(line, `${1}<removed>`)
		}
	}

	return line
}
