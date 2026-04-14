package parse

import (
	"regexp"
	"strings"
	"sync"
)

// ParsedConfig is the output of a device parser.
type ParsedConfig struct {
	Lines    []string          // filtered output lines written to VCS
	Metadata map[string]string // version, model, serial, etc.
}

// Parser is the interface every device parser must satisfy.
type Parser interface {
	// Parse processes raw device output (all commands combined) into a filtered config.
	Parse(output []byte, filter FilterOpts) (ParsedConfig, error)
}

// FilterOpts controls password and community-string filtering, matching RANCID's
// FILTER_PWDS / FILTER_OSC / NOCOMMSTR settings.
type FilterOpts struct {
	FilterPwds   int  // 0=no filtering, 1=filter clear text, 2=filter all (including encrypted)
	FilterOsc    int  // 0=no filtering, 1=filter some, 2=filter all oscillating values
	NoCommStr    bool // true = remove SNMP community strings
	ACLFilterSeq bool // true = strip ACL sequence numbers
	ACLFilterRe  *regexp.Regexp
}

// registry holds per-device-type parsers registered at init time.
var (
	mu      sync.RWMutex
	parsers = make(map[string]Parser)
)

// Register adds a parser for the given device type name.
// Typically called from an init() function in each parser package.
func Register(deviceType string, p Parser) {
	mu.Lock()
	defer mu.Unlock()
	parsers[strings.ToLower(deviceType)] = p
}

// RegisterAlias maps an additional device type name to an already-registered parser.
// It is used for upstream RANCID aliases and parser-compatible families.
func RegisterAlias(deviceType, target string) {
	mu.Lock()
	defer mu.Unlock()

	if p, ok := parsers[strings.ToLower(target)]; ok {
		parsers[strings.ToLower(deviceType)] = p
	}
}

// Lookup returns the parser for a device type, or false if none is registered.
func Lookup(deviceType string) (Parser, bool) {
	mu.RLock()
	defer mu.RUnlock()
	p, ok := parsers[strings.ToLower(deviceType)]
	return p, ok
}

// RegisteredTypes returns all device type names that have a Go parser.
func RegisteredTypes() []string {
	mu.RLock()
	defer mu.RUnlock()
	types := make([]string, 0, len(parsers))
	for t := range parsers {
		types = append(types, t)
	}
	return types
}
