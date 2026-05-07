// Package iosxr implements the RANCID parser for Cisco IOS-XR devices.
//
// It processes the combined output of "admin show version" and
// "show running-config", applying the same filtering rules as the
// original Perl iosxr.pm module: password/secret removal, volatile
// line stripping, timestamp removal, community-string filtering,
// and CVS-tag neutralisation.
package iosxr

import (
	"regexp"
	"strings"

	"gorancid/pkg/connect"
	"gorancid/pkg/parse"
)

// IOSXRParser implements parse.Parser for Cisco IOS-XR devices.
type IOSXRParser struct{}

func init() {
	parse.Register("iosxr", &IOSXRParser{})
}

// DeviceOpts returns connection parameters specific to IOS-XR devices.
func (p *IOSXRParser) DeviceOpts() connect.DeviceOpts {
	return connect.DeviceOpts{
		DeviceType:       "iosxr",
		PromptPattern:    `(?:^|[\r\n])[\w./:-]+(?:\([^)]+\))*#\s*$`,
		SetupCommands:    []string{"terminal length 0", "terminal width 0", "terminal no-timestamp"},
		EnableCmd:        "",
		DisablePagingCmd: "terminal length 0",
	}
}

// Pre-compiled regular expressions used during parsing.
var (
	// ShowVersion patterns
	reVersion       = regexp.MustCompile(`^(Cisco )?IOS .* Software,? \(([A-Za-z0-9_-]*)\), .*Version\s+(.*)$`)
	reSerial        = regexp.MustCompile(`(?i)processor board id (\S+)`)
	reSerial2       = regexp.MustCompile(`^Serial Number:\s+(.*)$`)
	reConfigReg     = regexp.MustCompile(`^Configuration register is (.*)$`)
	reConfigRegNode = regexp.MustCompile(`^Configuration register on node \S+ is (.*)$`)
	reBootImage     = regexp.MustCompile(`^System image file is "([^"]*)"`)

	// Common filter patterns (shared between ShowVersion and WriteTerm)
	reTimestamp = regexp.MustCompile(`^\w{3} \w{3,4} {1,3}\d{1,2} {1,2}\d{1,2}:\d+:\d+\.\d+ \S+$`)
	reNCSJunk   = regexp.MustCompile(`(?i)^(\x1b\[.\d+h)?sysadmin-vm:[^#]+# `)
	reNCSError  = regexp.MustCompile(`(?i)^(-+\^|syntax error: unknown argument)$`)

	// WriteTerm header / volatile lines
	reCmdHeader   = regexp.MustCompile(`^!Command: show running-config`)
	reTimeHeader  = regexp.MustCompile(`^!Time:`)
	reBuildingCfg = regexp.MustCompile(`^(?i)building configuration.*`)
	reNoChange    = regexp.MustCompile(`^! no configuration change since last restart`)
	reLastChange  = regexp.MustCompile(`^! (Last configuration|NVRAM config last)`)
	reWrittenBy   = regexp.MustCompile(`^: (Written by \S+ at|Saved)`)
	reNTPClock    = regexp.MustCompile(`^ntp clock-period `)
	reTFTPFlash   = regexp.MustCompile(`^tftp-server flash `)
	reFairQueue   = regexp.MustCompile(`fair-queue individual-limit`)
	reClockRate   = regexp.MustCompile(`^ clockrate `)

	// Password / secret filtering patterns
	reEnablePassword   = regexp.MustCompile(`^(enable )?(password|passwd)( level \d+)? `)
	reEnableSecret     = regexp.MustCompile(`^(enable secret) `)
	reUsernameSecret   = regexp.MustCompile(`^username (\S+)(\s.*)? secret `)
	reUsernamePassword = regexp.MustCompile(`^username (\S+)(\s.*)? password ((\d) \S+|\S+)`)
	reSessionKeyAH     = regexp.MustCompile(`^( set session-key (in|out)bound ah \d+ )`)
	reSessionKeyESP    = regexp.MustCompile(`^( set session-key (in|out)bound esp \d+ (authenticator|cypher) )`)
	reLinePassword     = regexp.MustCompile(`^(\s*)password `)
	reLineSecret       = regexp.MustCompile(`^(\s*)secret `)
	reBGPNeighborPwd   = regexp.MustCompile(`^\s*neighbor (\S*) password `)
	rePPPPassword      = regexp.MustCompile(`^(ppp .* password) 7 .*`)
	reFTPClientPwd     = regexp.MustCompile(`^(ftp client password) `)
	reOSPFAuthKey      = regexp.MustCompile(`^( ip ospf authentication-key) `)
	reISISPassword     = regexp.MustCompile(`^\s+isis password (\S+)( .*)?`)
	reISISDomainPwd    = regexp.MustCompile(`^\s+(domain-password|area-password) (\S+)( .*)?`)
	reOSPFDigestKey    = regexp.MustCompile(`^( ip ospf message-digest-key \d+ md5) `)
	reMD5Key           = regexp.MustCompile(`^(  message-digest-key \d+ md5 (7|encrypted)) `)
	reISAKMPKey        = regexp.MustCompile(`^((crypto )?isakmp key) \S+ `)
	reHSRPAuth         = regexp.MustCompile(`^(\s+standby \d+ authentication) `)
	reKeyString        = regexp.MustCompile(`^(\s+key-string \d?)`)
	reL2TPTunnel       = regexp.MustCompile(`^( l2tp tunnel \S+ password)`)
	reVPDNUsername     = regexp.MustCompile(`^(vpdn username (\S+) password)`)
	rePreSharedKey     = regexp.MustCompile(`^( pre-shared-key | key |failover key ).*`)
	reLDAPLoginPwd     = regexp.MustCompile(`(\s+ldap-login-password )\S+(.*)`)
	reCableShared      = regexp.MustCompile(`^( cable shared-secret )`)
	reTacacsKey        = regexp.MustCompile(`^((tacacs|radius)-server\s(\w*[-\s(\s\S+])*\s?key) (\d )?\w+`)
	reNTPAuthKey       = regexp.MustCompile(`^(ntp authentication-key \d+ md5) `)
	reSysconPwd        = regexp.MustCompile(`^syscon password (\S*)`)
	reSysconAddr       = regexp.MustCompile(`^syscon address (\S*) (\S*)`)

	// SNMP community filtering
	reSNMPCommunity = regexp.MustCompile(`^(snmp-server community) (\S+)`)
	reSNMPHost      = regexp.MustCompile(`^snmp-server host (\d+\.\d+\.\d+\.\d+) `)

	// CVS tag neutralisation
	reCVSTag = regexp.MustCompile(`\$(Revision|Id):`)
)

// Parse processes raw IOS-XR device output and returns a filtered configuration.
//
// The input is expected to be the concatenated output of the commands
// collected from the device (admin show version, show running-config, etc.).
// Parse splits the output on command boundaries, classifies each section,
// and applies the appropriate filtering rules.
func (p *IOSXRParser) Parse(output []byte, filter parse.FilterOpts) (parse.ParsedConfig, error) {
	text := strings.ReplaceAll(string(output), "\r", "")
	lines := strings.Split(text, "\n")

	meta := make(map[string]string)
	var filtered []string

	inVersion := false
	inConfig := false
	configRegister := ""
	commentCount := 0

	for i := 0; i < len(lines); i++ {
		line := lines[i]

		// ---- Detect section boundaries ----
		// "admin show version" or "show version"
		if strings.Contains(line, "show version") && !inVersion && !inConfig {
			inVersion = true
			continue
		}
		// "show running-config" starts the config section
		if strings.Contains(line, "show running-config") && !inConfig {
			inVersion = false
			inConfig = true
			continue
		}

		// ---- Common filters applied everywhere ----
		// Skip empty lines at boundaries
		if strings.TrimSpace(line) == "" {
			continue
		}
		// Command echo lines (just the command itself, already consumed above)
		if reTimestamp.MatchString(line) {
			continue
		}
		// NCS junk
		if reNCSJunk.MatchString(line) {
			continue
		}
		// Error markers
		if reNCSError.MatchString(line) {
			continue
		}
		if strings.HasPrefix(line, "Error:") {
			continue
		}
		if strings.Contains(line, "Invalid input detected") || strings.Contains(line, "Type help or ") {
			continue
		}
		if strings.Contains(line, "command authorization failed") {
			continue
		}
		if strings.Contains(line, "Line has invalid autocommand") {
			continue
		}

		// ---- ShowVersion section ----
		if inVersion {
			if processVersionLine(line, meta, &filtered, &configRegister) {
				continue
			}
			// Skip other version lines (don't add to config)
			continue
		}

		// ---- WriteTerm (show running-config) section ----
		if inConfig {
			appendLines, done := processConfigLine(line, filter, &commentCount)
			if done {
				filtered = append(filtered, appendLines...)
				break
			}
			if appendLines != nil {
				filtered = append(filtered, appendLines...)
			}
			continue
		}
	}

	// Inject config-register if extracted from version
	if configRegister != "" {
		// Prepend after any header lines
		cfgRegLine := "config-register " + configRegister
		// Insert after leading comment lines
		inserted := false
		var result []string
		for _, l := range filtered {
			if !inserted && !strings.HasPrefix(l, "!") {
				result = append(result, "!")
				result = append(result, cfgRegLine)
				inserted = true
			}
			result = append(result, l)
		}
		if !inserted {
			result = append(result, "!")
			result = append(result, cfgRegLine)
		}
		filtered = result
	}

	return parse.ParsedConfig{
		Lines:    filtered,
		Metadata: meta,
	}, nil
}

// reChassis matches the processor/chassis line from show version.
// IOS-XR devices report chassis info like:
//
//	"cisco ASR-9006 (...)
//	"cisco CRS-16/S (...)
var reChassis = regexp.MustCompile(`^(?i)cisco (\S+(?:\s+series)?)\s+\(`)
