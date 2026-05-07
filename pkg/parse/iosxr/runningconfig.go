package iosxr

import (
	"strings"

	"gorancid/pkg/parse"
)

// processConfigLine handles a single line from the running-config section.
// It applies header filters, volatile line removal, comment collapsing,
// password filtering, and returns the lines to append (or nil to skip).
// It also updates commentCount and breaks on "end".
func processConfigLine(line string, filter parse.FilterOpts, commentCount *int) (appendLines []string, done bool) {
	// End of config
	if line == "end" {
		return []string{"end"}, true
	}

	// Remove !Command: and !Time: header lines
	if reCmdHeader.MatchString(line) {
		return nil, false
	}
	if reTimeHeader.MatchString(line) {
		return nil, false
	}
	// Remove "Building configuration..." header
	if reBuildingCfg.MatchString(line) {
		return nil, false
	}
	// Remove "Current configuration : " header
	if strings.HasPrefix(line, "Current configuration") {
		return nil, false
	}
	// Remove "no configuration change" marker
	if reNoChange.MatchString(line) {
		return nil, false
	}
	// Remove "Last configuration change" and "Written by" / "Saved"
	if reLastChange.MatchString(line) {
		return nil, false
	}
	if reWrittenBy.MatchString(line) {
		return nil, false
	}
	// Remove NTP clock-period (volatile)
	if reNTPClock.MatchString(line) {
		return nil, false
	}
	// Remove tftp-server flash lines
	if reTFTPFlash.MatchString(line) {
		return nil, false
	}
	// Remove fair-queue individual-limit
	if reFairQueue.MatchString(line) {
		return nil, false
	}
	// Remove clockrate on serial interfaces
	if reClockRate.MatchString(line) {
		return nil, false
	}

	// Collapse consecutive comment (!) lines
	if strings.HasPrefix(line, "!") && strings.TrimSpace(line) == "!" {
		if *commentCount > 0 {
			return nil, false
		}
		*commentCount++
		return []string{line}, false
	}
	*commentCount = 0

	// Password filtering (may return 0 or more lines)
	return applyPasswordFilters(line, filter), false
}
