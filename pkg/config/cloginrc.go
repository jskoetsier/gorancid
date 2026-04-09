package config

import (
	"bufio"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// Credentials holds login credentials for a device.
type Credentials struct {
	Username  string
	Password  string
	EnablePwd string
	Methods   []string // e.g. ["ssh", "telnet"]
}

type credEntry struct {
	field   string // "user", "password", "enablepassword", "method"
	pattern string // glob pattern matched against hostname
	value   string
}

// CredStore holds parsed .cloginrc entries.
type CredStore struct {
	entries []credEntry
}

// entryRE matches: add <field> <pattern> <value>
// value may be wrapped in TCL braces: {ssh telnet}
var entryRE = regexp.MustCompile(`^add\s+(\S+)\s+(\S+)\s+(.+)$`)

// LoadCloginrc parses a .cloginrc file into a CredStore.
func LoadCloginrc(path string) (*CredStore, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	cs := &CredStore{}
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		m := entryRE.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		field := strings.ToLower(m[1])
		pattern := m[2]
		value := strings.Trim(strings.TrimSpace(m[3]), "{}")
		cs.entries = append(cs.entries, credEntry{field, pattern, value})
	}
	return cs, scanner.Err()
}

// isExactPattern returns true if the pattern contains no glob metacharacters.
func isExactPattern(pattern string) bool {
	return !strings.ContainsAny(pattern, "*?[")
}

// Lookup returns credentials for hostname.
// Exact-match patterns take priority over glob patterns; among patterns of
// equal specificity the first entry in file order wins.
func (cs *CredStore) Lookup(hostname string) Credentials {
	var creds Credentials
	// Track best match per field: exact wins over glob.
	type match struct {
		value string
		exact bool
	}
	best := make(map[string]match)
	for _, e := range cs.entries {
		matched, _ := filepath.Match(e.pattern, hostname)
		if !matched {
			continue
		}
		exact := isExactPattern(e.pattern)
		if prev, ok := best[e.field]; ok {
			// Already have a match; only replace if current is exact and prev is not.
			if prev.exact || !exact {
				continue
			}
		}
		best[e.field] = match{value: e.value, exact: exact}
	}
	if m, ok := best["user"]; ok {
		creds.Username = m.value
	}
	if m, ok := best["password"]; ok {
		creds.Password = m.value
	}
	if m, ok := best["enablepassword"]; ok {
		creds.EnablePwd = m.value
	}
	if m, ok := best["method"]; ok {
		creds.Methods = strings.Fields(m.value)
	}
	return creds
}
