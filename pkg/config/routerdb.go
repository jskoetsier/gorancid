package config

import (
	"bufio"
	"fmt"
	"os"
	"strings"
)

// Device represents a single entry in router.db.
type Device struct {
	Hostname string
	Type     string // key into devicetype registry, e.g. "ios"
	Status   string // "up" or "down"
}

// LoadRouterDB reads a router.db file and returns all entries including down devices.
// Callers filter by Status as needed.
// Format per line: hostname:type:status or hostname;type;status — lines starting with # are comments.
func LoadRouterDB(path string) ([]Device, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var devices []Device
	scanner := bufio.NewScanner(f)
	lineNum := 0
	for scanner.Scan() {
		lineNum++
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		parts := strings.SplitN(line, ":", 3)
		if len(parts) != 3 {
			parts = strings.SplitN(line, ";", 3)
		}
		if len(parts) != 3 {
			return nil, fmt.Errorf("%s:%d: expected hostname:type:status or hostname;type;status, got %q", path, lineNum, line)
		}
		devices = append(devices, Device{
			Hostname: strings.TrimSpace(parts[0]),
			Type:     strings.TrimSpace(parts[1]),
			Status:   strings.TrimSpace(parts[2]),
		})
	}
	return devices, scanner.Err()
}
