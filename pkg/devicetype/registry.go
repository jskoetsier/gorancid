package devicetype

import (
	"bufio"
	"fmt"
	"os"
	"strings"
	"time"
)

// Command maps a CLI string to a handler name from the device's Perl/Go module.
type Command struct {
	CLI     string // e.g. "show version"
	Handler string // e.g. "ios::ShowVersion"
}

// DeviceSpec describes how to collect from a specific device type.
type DeviceSpec struct {
	Type        string
	Alias       string    // if non-empty, this type redirects to another
	Script      string    // e.g. "rancid -t ios"
	LoginScript string    // e.g. "clogin"
	Modules     []string  // module names (informational in Phase 1)
	InLoop      string    // inloop function name
	Commands    []Command // ordered list of commands to run
	Timeout     time.Duration
}

// Load reads rancid.types.base then rancid.types.conf.
// Types defined in base are NOT overridden by conf (upstream behavior).
// Types only in conf are added normally.
func Load(baseFile, confFile string) (map[string]DeviceSpec, error) {
	specs := make(map[string]DeviceSpec)
	baseTypes := make(map[string]bool)

	if err := loadFile(baseFile, specs, nil); err != nil {
		return nil, fmt.Errorf("%s: %w", baseFile, err)
	}
	for t := range specs {
		baseTypes[t] = true
	}
	if err := loadFile(confFile, specs, baseTypes); err != nil && !os.IsNotExist(err) {
		return nil, fmt.Errorf("%s: %w", confFile, err)
	}
	return specs, nil
}

// Lookup resolves a device type by name, following aliases.
// Returns the resolved DeviceSpec and true, or zero value and false.
func Lookup(specs map[string]DeviceSpec, devtype string) (DeviceSpec, bool) {
	seen := make(map[string]bool)
	t := strings.ToLower(devtype)
	for {
		if seen[t] {
			return DeviceSpec{}, false // alias cycle
		}
		seen[t] = true
		spec, ok := specs[t]
		if !ok {
			return DeviceSpec{}, false
		}
		if spec.Alias == "" {
			return spec, true
		}
		t = spec.Alias
	}
}

func loadFile(path string, specs map[string]DeviceSpec, skip map[string]bool) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()

	lineNum := 0
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		lineNum++
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		parts := strings.Split(line, ";")
		if len(parts) < 3 {
			return fmt.Errorf("line %d: too few fields in %q", lineNum, line)
		}
		devtype := strings.ToLower(strings.TrimSpace(parts[0]))
		directive := strings.ToLower(strings.TrimSpace(parts[1]))
		value := strings.TrimSpace(parts[2])

		if skip != nil && skip[devtype] {
			continue
		}

		spec := specs[devtype]
		spec.Type = devtype

		switch directive {
		case "script":
			spec.Script = value
		case "login":
			spec.LoginScript = value
		case "module":
			spec.Modules = append(spec.Modules, value)
		case "inloop":
			spec.InLoop = value
		case "alias":
			spec.Alias = strings.ToLower(value)
		case "timeout":
			var secs float64
			fmt.Sscanf(value, "%f", &secs)
			spec.Timeout = time.Duration(secs * float64(time.Second))
		case "command":
			if len(parts) < 4 {
				return fmt.Errorf("line %d: command directive needs 4 fields", lineNum)
			}
			cli := strings.TrimSpace(parts[3])
			spec.Commands = append(spec.Commands, Command{CLI: cli, Handler: value})
		}
		specs[devtype] = spec
	}
	return scanner.Err()
}