package iosxr

import "strings"

// processVersionLine handles a single line from the show version section.
// It updates metadata and filtered output, returns true if the line was consumed.
func processVersionLine(line string, meta map[string]string, filtered *[]string, configRegister *string) bool {
	// Version / image
	if m := reVersion.FindStringSubmatch(line); m != nil {
		meta["image"] = m[2]
		meta["version"] = m[3]
		*filtered = append(*filtered, "!Image: Software: "+m[2]+", "+m[3])
		return true
	}
	// Processor board serial
	if m := reSerial.FindStringSubmatch(line); m != nil {
		sn := strings.TrimRight(m[1], ",")
		meta["serial"] = sn
		*filtered = append(*filtered, "!Processor ID: "+sn)
		return true
	}
	// Alternate serial format
	if m := reSerial2.FindStringSubmatch(line); m != nil {
		meta["serial"] = strings.TrimSpace(m[1])
		*filtered = append(*filtered, "!Serial Number: "+strings.TrimSpace(m[1]))
		return true
	}
	// Config register
	if m := reConfigReg.FindStringSubmatch(line); m != nil {
		*configRegister = m[1]
		meta["config_register"] = m[1]
		return true
	}
	if m := reConfigRegNode.FindStringSubmatch(line); m != nil {
		if *configRegister == "" {
			*configRegister = m[1]
			meta["config_register"] = m[1]
		}
		return true
	}
	// Boot image
	if m := reBootImage.FindStringSubmatch(line); m != nil {
		meta["boot_image"] = m[1]
		*filtered = append(*filtered, "!Image: "+m[1])
		return true
	}
	// Processor/chassis detection
	if m := reChassis.FindStringSubmatch(line); m != nil {
		meta["processor"] = m[1]
		*filtered = append(*filtered, "!Chassis type: "+m[1])
		return true
	}
	return false
}
