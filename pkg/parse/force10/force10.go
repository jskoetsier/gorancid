// Package force10 implements the Dell Force10 / DNOS9 (Dell NOS9) device parser
// for gorancid. This covers Force10, Dell Force10, and DNOS9 aliases.
// Commands collected: show version, show bootvar, dir flash:, dir slot0:,
// show chassis, show system, show inventory, show vlan, show running.
package force10

import (
	"bufio"
	"bytes"
	"regexp"
	"strings"

	"gorancid/pkg/connect"
	"gorancid/pkg/parse"
)

func init() {
	parse.Register("dnos9", &Force10Parser{})
	parse.RegisterAlias("force10", "dnos9")
	parse.RegisterAlias("dell", "dnos9")
}

// Force10Parser implements parse.Parser for Dell Force10 / DNOS9 devices.
type Force10Parser struct{}

// DeviceOpts returns connection parameters for Force10 FTOS.
// FTOS prompt: "hostname#" in exec mode, "hostname(conf)#" in config mode.
func (p *Force10Parser) DeviceOpts() connect.DeviceOpts {
	return connect.DeviceOpts{
		DeviceType:       "force10",
		PromptPattern:    `(?:^|[\r\n])[\w./-]+(?:\([^)]+\))?#\s*$`,
		SetupCommands:    []string{"terminal length 0", "terminal width 132"},
		EnableCmd:        "enable",
		DisablePagingCmd: "terminal length 0",
	}
}

// ---------------------------------------------------------------------------
// Section tracking
// ---------------------------------------------------------------------------

type section int

const (
	sectionNone section = iota
	sectionShowVersion
	sectionShowBootvar
	sectionDirFlash
	sectionDirSlot
	sectionShowChassis
	sectionShowSystem
	sectionShowInventory
	sectionShowVLAN
	sectionShowRunning
)

// ---------------------------------------------------------------------------
// Parse
// ---------------------------------------------------------------------------

func (p *Force10Parser) Parse(output []byte, filter parse.FilterOpts) (parse.ParsedConfig, error) {
	md := make(map[string]string)
	var lines []string
	sec := sectionNone
	prevWasBang := false

	scanner := bufio.NewScanner(bytes.NewReader(output))
	scanner.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)

	for scanner.Scan() {
		line := scanner.Text()
		line = strings.TrimRight(line, "\r")

		if s, ok := detectSection(line); ok {
			sec = s
			prevWasBang = false
			continue
		}

		switch sec {
		case sectionShowVersion:
			if l := processShowVersionLine(line, md); l != "" {
				lines = append(lines, l)
			}
		case sectionShowBootvar, sectionDirFlash, sectionDirSlot,
			sectionShowChassis, sectionShowSystem, sectionShowInventory, sectionShowVLAN:
			if !isVolatileLine(line) && strings.TrimSpace(line) != "" {
				lines = append(lines, line)
			}
		case sectionShowRunning:
			if l := processRunningLine(line, filter, &prevWasBang); l != "" {
				lines = append(lines, l)
			}
		}
	}

	return parse.ParsedConfig{Lines: lines, Metadata: md}, nil
}

// ---------------------------------------------------------------------------
// Command / section detection
// ---------------------------------------------------------------------------

var rePromptPrefix = regexp.MustCompile(`^[\w./-]+(?:\([^)]+\))?#\s*`)

var sectionCommands = []struct {
	pattern *regexp.Regexp
	sec     section
}{
	{regexp.MustCompile(`(?i)^show\s+version$`), sectionShowVersion},
	{regexp.MustCompile(`(?i)^show\s+bootvar$`), sectionShowBootvar},
	{regexp.MustCompile(`(?i)^dir\s+flash:`), sectionDirFlash},
	{regexp.MustCompile(`(?i)^dir\s+slot\d*:`), sectionDirSlot},
	{regexp.MustCompile(`(?i)^show\s+chassis$`), sectionShowChassis},
	{regexp.MustCompile(`(?i)^show\s+system$`), sectionShowSystem},
	{regexp.MustCompile(`(?i)^show\s+inventory$`), sectionShowInventory},
	{regexp.MustCompile(`(?i)^show\s+vlan$`), sectionShowVLAN},
	{regexp.MustCompile(`(?i)^show\s+running(?:-config)?$`), sectionShowRunning},
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
// show version
// ---------------------------------------------------------------------------

var (
	reVersion   = regexp.MustCompile(`(?i)Dell (?:Networking|Force10) OS.*Version\s+(\S+)`)
	reSerial    = regexp.MustCompile(`(?i)system serial number\s*:\s*(\S+)`)
	reModel     = regexp.MustCompile(`(?i)system type\s*:\s*(.+)`)
	reUptime    = regexp.MustCompile(`(?i)system up time|uptime`)
	rePager     = regexp.MustCompile(`--\s*[Mm]ore\s*--`)
)

func processShowVersionLine(line string, md map[string]string) string {
	if reUptime.MatchString(line) || rePager.MatchString(line) {
		return ""
	}
	if strings.TrimSpace(line) == "" {
		return ""
	}
	if m := reVersion.FindStringSubmatch(line); m != nil {
		md["version"] = strings.TrimSpace(m[1])
	}
	if m := reSerial.FindStringSubmatch(line); m != nil {
		md["serial"] = strings.TrimSpace(m[1])
	}
	if m := reModel.FindStringSubmatch(line); m != nil {
		md["model"] = strings.TrimSpace(m[1])
	}
	return line
}

// ---------------------------------------------------------------------------
// Volatile filtering for show chassis / system / inventory / vlan
// ---------------------------------------------------------------------------

var reVolatile = regexp.MustCompile(
	`(?i)(?:up time|uptime|temperature|fan speed|current time|system time|last boot)`,
)

func isVolatileLine(line string) bool {
	return reVolatile.MatchString(line) || rePager.MatchString(line)
}

// ---------------------------------------------------------------------------
// show running-config
// ---------------------------------------------------------------------------

var (
	reRCSTag        = regexp.MustCompile(`\$Revision:`)
	reIdTag         = regexp.MustCompile(`\$Id:`)
	reTimestamp     = regexp.MustCompile(`(?i)^!\s+(?:last configuration change|NTP time|system time)`)
	// Password patterns
	reEnablePassword    = regexp.MustCompile(`^(\s*enable (?:password|secret))\s+\S+(.*)`)
	reUsernamePassword  = regexp.MustCompile(`^(\s*username\s+\S+\s+(?:password|secret))\s+\S+(.*)`)
	reBgpPassword       = regexp.MustCompile(`^(\s*neighbor\s+\S+\s+password)\s+\S+(.*)`)
	reOspfAuthKey       = regexp.MustCompile(`^(\s*ip ospf authentication-key)\s+\S+(.*)`)
	reSnmpCommunity     = regexp.MustCompile(`^(\s*snmp-server community)\s+\S+(.*)`)
)

func processRunningLine(line string, filter parse.FilterOpts, prevWasBang *bool) string {
	if reTimestamp.MatchString(line) {
		return ""
	}
	if rePager.MatchString(line) {
		return ""
	}
	if reRCSTag.MatchString(line) {
		line = strings.ReplaceAll(line, "$Revision:", "Revision:")
	}
	if reIdTag.MatchString(line) {
		line = strings.ReplaceAll(line, "$Id:", "Id:")
	}

	// Collapse consecutive "!" separators.
	if strings.TrimSpace(line) == "!" {
		if *prevWasBang {
			return ""
		}
		*prevWasBang = true
		return line
	}
	*prevWasBang = false

	if filter.NoCommStr {
		if m := reSnmpCommunity.FindStringSubmatch(line); m != nil {
			if len(m) >= 3 {
				return m[1] + " <removed>" + m[2]
			}
			return m[1] + " <removed>"
		}
	}

	if filter.FilterPwds >= 1 {
		for _, pat := range []*regexp.Regexp{reEnablePassword, reUsernamePassword, reBgpPassword, reOspfAuthKey} {
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
