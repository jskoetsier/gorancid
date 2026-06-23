// Package fortigatealias identifies RANCID device type names that should use
// the FortiGate collector and parser when no dedicated type entry exists.
package fortigatealias

import "strings"

// UsesFortiGate reports whether devtype should inherit the fortigate spec/parser.
// Matching is intentionally narrow: Observium's fortiscp and explicit fortigate-* aliases,
// not every Fortinet product OS name (fortiswitch, forti-web, etc.).
func UsesFortiGate(devtype string) bool {
	_, ok := BaseType(devtype)
	return ok
}

// BaseType returns "fortigate" when devtype is a known FortiGate-family alias.
func BaseType(devtype string) (string, bool) {
	t := strings.ToLower(strings.TrimSpace(devtype))
	switch t {
	case "fortigate-full":
		return "fortigate", true
	}
	if strings.HasPrefix(t, "forti") && strings.HasSuffix(t, "scp") {
		return "fortigate", true
	}
	return "", false
}
