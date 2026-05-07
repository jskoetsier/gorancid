package iosxr

import (
	"strings"

	"gorancid/pkg/parse"
)

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
