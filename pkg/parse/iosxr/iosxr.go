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
		PromptPattern:    `[\r\n][\w./-:\-]+#\s*$`,
		SetupCommands:    []string{"terminal length 0", "terminal width 0", "terminal no-timestamp"},
		EnableCmd:        "",
		DisablePagingCmd: "terminal length 0",
	}
}

// Pre-compiled regular expressions used during parsing.
var (
	// ShowVersion patterns
	reVersion = regexp.MustCompile(`^(Cisco )?IOS .* Software,? \(([A-Za-z0-9_-]*)\), .*Version\s+(.*)$`)
	reSerial  = regexp.MustCompile(`(?i)processor board id (\S+)`)
	reSerial2 = regexp.MustCompile(`^Serial Number:\s+(.*)$`)
	reConfigReg = regexp.MustCompile(`^Configuration register is (.*)$`)
	reConfigRegNode = regexp.MustCompile(`^Configuration register on node \S+ is (.*)$`)
	reBootImage = regexp.MustCompile(`^System image file is "([^"]*)"`)

	// Common filter patterns (shared between ShowVersion and WriteTerm)
	reTimestamp = regexp.MustCompile(`^\w{3} \w{3,4} {1,3}\d{1,2} {1,2}\d{1,2}:\d+:\d+\.\d+ \S+$`)
	reNCSJunk   = regexp.MustCompile(`(?i)^(\x1b\[.\d+h)?sysadmin-vm:[^#]+# `)
	reNCSError  = regexp.MustCompile(`(?i)^(-+\^|syntax error: unknown argument)$`)

	// WriteTerm header / volatile lines
	reCmdHeader    = regexp.MustCompile(`^!Command: show running-config`)
	reTimeHeader   = regexp.MustCompile(`^!Time:`)
	reBuildingCfg = regexp.MustCompile(`^(?i)building configuration.*`)
	reNoChange    = regexp.MustCompile(`^! no configuration change since last restart`)
	reLastChange  = regexp.MustCompile(`^! (Last configuration|NVRAM config last)`)
	reWrittenBy    = regexp.MustCompile(`^: (Written by \S+ at|Saved)`)
	reNTPClock     = regexp.MustCompile(`^ntp clock-period `)
	reTFTPFlash    = regexp.MustCompile(`^tftp-server flash `)
	reFairQueue    = regexp.MustCompile(`fair-queue individual-limit`)
	reClockRate    = regexp.MustCompile(`^ clockrate `)

	// Password / secret filtering patterns
	reEnablePassword = regexp.MustCompile(`^(enable )?(password|passwd)( level \d+)? `)
	reEnableSecret   = regexp.MustCompile(`^(enable secret) `)
	reUsernameSecret = regexp.MustCompile(`^username (\S+)(\s.*)? secret `)
	reUsernamePassword = regexp.MustCompile(`^username (\S+)(\s.*)? password ((\d) \S+|\S+)`)
	reSessionKeyAH   = regexp.MustCompile(`^( set session-key (in|out)bound ah \d+ )`)
	reSessionKeyESP  = regexp.MustCompile(`^( set session-key (in|out)bound esp \d+ (authenticator|cypher) )`)
	reLinePassword   = regexp.MustCompile(`^(\s*)password `)
	reLineSecret     = regexp.MustCompile(`^(\s*)secret `)
	reBGPNeighborPwd = regexp.MustCompile(`^\s*neighbor (\S*) password `)
	rePPPPassword    = regexp.MustCompile(`^(ppp .* password) 7 .*`)
	reFTPClientPwd   = regexp.MustCompile(`^(ftp client password) `)
	reOSPFAuthKey    = regexp.MustCompile(`^( ip ospf authentication-key) `)
	reISISPassword   = regexp.MustCompile(`^\s+isis password (\S+)( .*)?`)
	reISISDomainPwd  = regexp.MustCompile(`^\s+(domain-password|area-password) (\S+)( .*)?`)
	reOSPFDigestKey  = regexp.MustCompile(`^( ip ospf message-digest-key \d+ md5) `)
	reMD5Key         = regexp.MustCompile(`^(  message-digest-key \d+ md5 (7|encrypted)) `)
	reISAKMPKey      = regexp.MustCompile(`^((crypto )?isakmp key) \S+ `)
	reHSRPAuth       = regexp.MustCompile(`^(\s+standby \d+ authentication) `)
	reKeyString      = regexp.MustCompile(`^(\s+key-string \d?)`)
	reL2TPTunnel     = regexp.MustCompile(`^( l2tp tunnel \S+ password)`)
	reVPDNUsername   = regexp.MustCompile(`^(vpdn username (\S+) password)`)
	rePreSharedKey   = regexp.MustCompile(`^( pre-shared-key | key |failover key ).*`)
	reLDAPLoginPwd   = regexp.MustCompile(`(\s+ldap-login-password )\S+(.*)`)
	reCableShared    = regexp.MustCompile(`^( cable shared-secret )`)
	reTacacsKey      = regexp.MustCompile(`^((tacacs|radius)-server\s(\w*[-\s(\s\S+])*\s?key) (\d )?\w+`)
	reNTPAuthKey     = regexp.MustCompile(`^(ntp authentication-key \d+ md5) `)
	reSysconPwd      = regexp.MustCompile(`^syscon password (\S*)`)
	reSysconAddr     = regexp.MustCompile(`^syscon address (\S*) (\S*)`)

	// SNMP community filtering
	reSNMPCommunity = regexp.MustCompile(`^(snmp-server community) (\S+)`)
	reSNMPHost     = regexp.MustCompile(`^snmp-server host (\d+\.\d+\.\d+\.\d+) `)

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
			// Version / image
			if m := reVersion.FindStringSubmatch(line); m != nil {
				meta["image"] = m[2]
				meta["version"] = m[3]
				filtered = append(filtered, "!Image: Software: "+m[2]+", "+m[3])
				continue
			}
			// Processor board serial
			if m := reSerial.FindStringSubmatch(line); m != nil {
				sn := strings.TrimRight(m[1], ",")
				meta["serial"] = sn
				filtered = append(filtered, "!Processor ID: "+sn)
				continue
			}
			// Alternate serial format
			if m := reSerial2.FindStringSubmatch(line); m != nil {
				meta["serial"] = strings.TrimSpace(m[1])
				filtered = append(filtered, "!Serial Number: "+strings.TrimSpace(m[1]))
				continue
			}
			// Config register
			if m := reConfigReg.FindStringSubmatch(line); m != nil {
				configRegister = m[1]
				meta["config_register"] = m[1]
				continue
			}
			if m := reConfigRegNode.FindStringSubmatch(line); m != nil {
				if configRegister == "" {
					configRegister = m[1]
					meta["config_register"] = m[1]
				}
				continue
			}
			// Boot image
			if m := reBootImage.FindStringSubmatch(line); m != nil {
				meta["boot_image"] = m[1]
				filtered = append(filtered, "!Image: "+m[1])
				continue
			}
			// Processor/chassis detection
			if m := reChassis.FindStringSubmatch(line); m != nil {
				meta["processor"] = m[1]
				filtered = append(filtered, "!Chassis type: "+m[1])
				continue
			}

			// Skip other version lines (don't add to config)
			continue
		}

		// ---- WriteTerm (show running-config) section ----
		if inConfig {
			// End of config
			if line == "end" {
				filtered = append(filtered, "end")
				break
			}

			// Remove !Command: and !Time: header lines
			if reCmdHeader.MatchString(line) {
				continue
			}
			if reTimeHeader.MatchString(line) {
				continue
			}
			// Remove "Building configuration..." header
			if reBuildingCfg.MatchString(line) {
				continue
			}
			// Remove "Current configuration : " header
			if strings.HasPrefix(line, "Current configuration") {
				continue
			}
			// Remove "no configuration change" marker
			if reNoChange.MatchString(line) {
				continue
			}
			// Remove "Last configuration change" and "Written by" / "Saved"
			if reLastChange.MatchString(line) {
				continue
			}
			if reWrittenBy.MatchString(line) {
				continue
			}
			// Remove NTP clock-period (volatile)
			if reNTPClock.MatchString(line) {
				continue
			}
			// Remove tftp-server flash lines
			if reTFTPFlash.MatchString(line) {
				continue
			}
			// Remove fair-queue individual-limit
			if reFairQueue.MatchString(line) {
				continue
			}
			// Remove clockrate on serial interfaces
			if reClockRate.MatchString(line) {
				continue
			}

			// Collapse consecutive comment (!) lines
			if strings.HasPrefix(line, "!") && strings.TrimSpace(line) == "!" {
				if commentCount > 0 {
					continue
				}
				commentCount++
				filtered = append(filtered, line)
				continue
			}
			commentCount = 0

			// ---- Password filtering ----
			filtered = append(filtered, applyPasswordFilters(line, filter)...)
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
//   "cisco ASR-9006 (...)
//   "cisco CRS-16/S (...)
var reChassis = regexp.MustCompile(`^(?i)cisco (\S+(?:\s+series)?)\s+\(`)

// applyPasswordFilters applies all password/secret/community filtering rules
// to a single config line. It returns zero or more output lines (usually one,
// but can be zero if the line is completely suppressed).
func applyPasswordFilters(line string, filter parse.FilterOpts) []string {
	fp := filter.FilterPwds

	// ---- Level 1 (clear text) and Level 2 (all) password filters ----

	// Enable password / passwd
	if m := reEnablePassword.FindStringSubmatch(line); m != nil {
		if fp >= 1 {
			return []string{"!" + m[1] + m[2] + m[3] + " <removed>"}
		}
		return []string{line}
	}

	// Enable secret
	if m := reEnableSecret.FindStringSubmatch(line); m != nil {
		if fp >= 2 {
			return []string{"!" + m[1] + " <removed>"}
		}
		return []string{line}
	}

	// Username secret
	if m := reUsernameSecret.FindStringSubmatch(line); m != nil {
		if fp >= 2 {
			return []string{"!username " + m[1] + m[2] + " secret <removed>"}
		}
		return []string{line}
	}

	// Username password
	if m := reUsernamePassword.FindStringSubmatch(line); m != nil {
		// m[4] is the encryption type digit (optional)
		if fp >= 2 {
			return []string{"!username " + m[1] + m[2] + " password <removed>"}
		} else if fp >= 1 && m[4] != "5" {
			return []string{"!username " + m[1] + m[2] + " password <removed>"}
		}
		return []string{line}
	}

	// IPSec session keys (AH)
	if m := reSessionKeyAH.FindStringSubmatch(line); m != nil {
		if fp >= 1 {
			return []string{"!" + m[1] + "<removed>"}
		}
		return []string{line}
	}

	// IPSec session keys (ESP)
	if m := reSessionKeyESP.FindStringSubmatch(line); m != nil {
		if fp >= 1 {
			return []string{"!" + m[1] + "<removed>"}
		}
		return []string{line}
	}

	// Line password
	if m := reLinePassword.FindStringSubmatch(line); m != nil {
		if fp >= 1 {
			return []string{"!" + m[1] + "password <removed>"}
		}
		return []string{line}
	}

	// Line secret
	if m := reLineSecret.FindStringSubmatch(line); m != nil {
		if fp >= 2 {
			return []string{"!" + m[1] + "secret <removed>"}
		}
		return []string{line}
	}

	// BGP neighbor password
	if m := reBGPNeighborPwd.FindStringSubmatch(line); m != nil {
		if fp >= 1 {
			return []string{"! neighbor " + m[1] + " password <removed>"}
		}
		return []string{line}
	}

	// PPP password
	if m := rePPPPassword.FindStringSubmatch(line); m != nil {
		if fp >= 1 {
			return []string{"!" + m[1] + " <removed>"}
		}
		return []string{line}
	}

	// FTP client password
	if m := reFTPClientPwd.FindStringSubmatch(line); m != nil {
		if fp >= 1 {
			return []string{"!" + m[1] + " <removed>"}
		}
		return []string{line}
	}

	// OSPF authentication key
	if m := reOSPFAuthKey.FindStringSubmatch(line); m != nil {
		if fp >= 1 {
			return []string{"!" + m[1] + " <removed>"}
		}
		return []string{line}
	}

	// ISIS password
	if m := reISISPassword.FindStringSubmatch(line); m != nil {
		if fp >= 1 {
			return []string{"!isis password <removed>" + m[2]}
		}
		return []string{line}
	}

	// ISIS domain/area password
	if m := reISISDomainPwd.FindStringSubmatch(line); m != nil {
		if fp >= 1 {
			return []string{"!" + m[1] + " <removed>" + m[3]}
		}
		return []string{line}
	}

	// OSPF message-digest-key (interface level)
	if m := reOSPFDigestKey.FindStringSubmatch(line); m != nil {
		if fp >= 1 {
			return []string{"!" + m[1] + " <removed>"}
		}
		return []string{line}
	}

	// Message-digest-key (router level)
	if m := reMD5Key.FindStringSubmatch(line); m != nil {
		if fp >= 1 {
			return []string{"!" + m[1] + " <removed>"}
		}
		return []string{line}
	}

	// ISakmp key
	if m := reISAKMPKey.FindStringSubmatch(line); m != nil {
		if fp >= 1 {
			rest := reISAKMPKey.ReplaceAllString(line, "")
			return []string{"!" + m[1] + " <removed> " + strings.TrimSpace(rest)}
		}
		return []string{line}
	}

	// HSRP authentication
	if m := reHSRPAuth.FindStringSubmatch(line); m != nil {
		if fp >= 1 {
			return []string{"!" + m[1] + " <removed>"}
		}
		return []string{line}
	}

	// IP SLA key-string
	if m := reKeyString.FindStringSubmatch(line); m != nil {
		if fp >= 1 {
			return []string{"!" + m[1] + " <removed>"}
		}
		return []string{line}
	}

	// L2TP tunnel password
	if m := reL2TPTunnel.FindStringSubmatch(line); m != nil {
		if fp >= 1 {
			return []string{"!" + m[1] + " <removed>"}
		}
		return []string{line}
	}

	// VPDN username password
	if m := reVPDNUsername.FindStringSubmatch(line); m != nil {
		if fp >= 1 {
			return []string{"!" + m[1] + " <removed>"}
		}
		return []string{line}
	}

	// Pre-shared-key / key / failover key
	if m := rePreSharedKey.FindStringSubmatch(line); m != nil {
		if fp >= 1 {
			// Find the remainder after the matched prefix
			idx := strings.Index(line, m[1]) + len(m[1])
			rest := ""
			if idx < len(line) {
				rest = line[idx:]
			}
			return []string{"!" + m[1] + " <removed> " + rest}
		}
		return []string{line}
	}

	// LDAP login password
	if m := reLDAPLoginPwd.FindStringSubmatch(line); m != nil {
		if fp >= 1 {
			return []string{"!" + m[1] + " <removed>" + m[2]}
		}
		return []string{line}
	}

	// Cable shared-secret
	if m := reCableShared.FindStringSubmatch(line); m != nil {
		if fp >= 1 {
			return []string{"!" + m[1] + " <removed>"}
		}
		return []string{line}
	}

	// TACACS/RADIUS server key
	if m := reTacacsKey.FindStringSubmatch(line); m != nil {
		if fp >= 1 {
			rest := reTacacsKey.ReplaceAllString(line, "")
			return []string{"!" + m[1] + " <removed>" + rest}
		}
		return []string{line}
	}

	// NTP authentication-key
	if m := reNTPAuthKey.FindStringSubmatch(line); m != nil {
		if fp >= 1 {
			return []string{"!" + m[1] + " <removed>"}
		}
		return []string{line}
	}

	// Syscon password
	if m := reSysconPwd.FindStringSubmatch(line); m != nil {
		if fp >= 1 {
			return []string{"!syscon password <removed>"}
		}
		return []string{line}
	}

	// Syscon address (always filter password portion)
	if m := reSysconAddr.FindStringSubmatch(line); m != nil {
		return []string{"!syscon address " + m[1] + " <removed>"}
	}

	// ---- SNMP community string filtering ----
	if m := reSNMPCommunity.FindStringSubmatch(line); m != nil {
		if filter.NoCommStr {
			// Replace community string with <removed>
			rest := reSNMPCommunity.ReplaceAllString(line, "")
			return []string{"!" + m[1] + " <removed>" + rest}
		}
		return []string{line}
	}

	// SNMP host community filtering
	if m := reSNMPHost.FindStringSubmatch(line); m != nil {
		if filter.NoCommStr {
			return filterSNMPHost(line, m[1])
		}
		return []string{line}
	}

	// ---- CVS tag neutralisation ----
	if strings.Contains(line, "$Revision:") || strings.Contains(line, "$Id:") {
		line = reCVSTag.ReplaceAllString(line, " $1:")
	}

	return []string{line}
}

// filterSNMPHost removes the community string from an
// "snmp-server host A.B.C.D <community> ..." line.
func filterSNMPHost(line, ip string) []string {
	prefix := "snmp-server host " + ip
	idx := strings.Index(line, prefix)
	if idx < 0 {
		return []string{line}
	}
	rest := line[idx+len(prefix):]
	tokens := strings.Fields(rest)

	var out strings.Builder
	out.WriteString(prefix)

	i := 0
	for i < len(tokens) {
		switch tokens[i] {
		case "version":
			out.WriteString(" " + tokens[i])
			i++
			if i < len(tokens) {
				out.WriteString(" " + tokens[i])
				// version 3 has an additional sub-token
				if tokens[i] == "3" && i+1 < len(tokens) {
					i++
					out.WriteString(" " + tokens[i])
				}
			}
		case "vrf":
			out.WriteString(" " + tokens[i])
			i++
			if i < len(tokens) {
				out.WriteString(" " + tokens[i])
			}
		case "informs", "traps", "noauth", "auth":
			out.WriteString(" " + tokens[i])
		default:
			// This token is the community string
			result := "!" + out.String() + " <removed>"
			// Append remaining tokens after the community
			for j := i + 1; j < len(tokens); j++ {
				result += " " + tokens[j]
			}
			return []string{result}
		}
		i++
	}
	return []string{out.String()}
}