package config

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Credentials holds login credentials for a device.
type Credentials struct {
	Username  string
	Password  string
	UserPwd   string
	EnablePwd string
	Methods   []string // e.g. ["ssh", "telnet"]
}

type credEntry struct {
	field   string // "user", "password", "enablepassword", "method"
	pattern string // glob pattern matched against hostname
	values  []string
}

// CredStore holds parsed .cloginrc entries.
type CredStore struct {
	entries []credEntry
}

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
		fields, err := splitCloginFields(line)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", path, err)
		}
		if len(fields) < 4 || strings.ToLower(fields[0]) != "add" {
			continue
		}
		field := strings.ToLower(fields[1])
		pattern := fields[2]
		values := append([]string(nil), fields[3:]...)
		cs.entries = append(cs.entries, credEntry{field: field, pattern: pattern, values: values})
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
			creds.Username = firstValue(e.values)
		case "userpassword":
			creds.UserPwd = firstValue(e.values)
			creds.Password = creds.UserPwd
		case "password":
			creds.Password = firstValue(e.values)
			if creds.EnablePwd == "" && len(e.values) > 1 {
				creds.EnablePwd = e.values[1]
			}
		case "enablepassword":
			creds.EnablePwd = firstValue(e.values)
		case "method":
			creds.Methods = expandMethodValues(e.values)
		}
	}
	if creds.Password == "" && creds.UserPwd != "" {
		creds.Password = creds.UserPwd
	}
	return creds
}

func firstValue(values []string) string {
	if len(values) == 0 {
		return ""
	}
	return values[0]
}

func expandMethodValues(values []string) []string {
	var methods []string
	for _, value := range values {
		methods = append(methods, strings.Fields(value)...)
	}
	return methods
}

func splitCloginFields(line string) ([]string, error) {
	var (
		fields []string
		buf    strings.Builder
		quote  rune
		depth  int
	)

	flush := func() {
		if buf.Len() == 0 {
			return
		}
		fields = append(fields, buf.String())
		buf.Reset()
	}

	for _, r := range line {
		switch {
		case quote != 0:
			if r == quote {
				quote = 0
			} else {
				buf.WriteRune(r)
			}
		case depth > 0:
			switch r {
			case '{':
				depth++
				buf.WriteRune(r)
			case '}':
				depth--
				if depth > 0 {
					buf.WriteRune(r)
				}
			default:
				buf.WriteRune(r)
			}
		case r == '\'' || r == '"':
			quote = r
		case r == '{':
			depth = 1
		case r == '#':
			flush()
			return fields, nil
		case r == ' ' || r == '\t':
			flush()
		default:
			buf.WriteRune(r)
		}
	}

	if quote != 0 || depth != 0 {
		return nil, fmt.Errorf("unterminated quoted or braced value")
	}
	flush()
	return fields, nil
}
