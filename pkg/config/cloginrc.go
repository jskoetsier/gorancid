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

// Lookup returns credentials for hostname.
// The first matching entry per field wins — place specific patterns before
// wildcards in .cloginrc to ensure correct precedence.
func (cs *CredStore) Lookup(hostname string) Credentials {
	var creds Credentials
	found := make(map[string]bool)
	for _, e := range cs.entries {
		if found[e.field] {
			continue
		}
		matched, _ := filepath.Match(e.pattern, hostname)
		if !matched {
			continue
		}
		found[e.field] = true
		switch e.field {
		case "user":
			creds.Username = e.value
		case "password":
			creds.Password = e.value
		case "enablepassword":
			creds.EnablePwd = e.value
		case "method":
			creds.Methods = strings.Fields(e.value)
		}
	}
	return creds
}
