package generic

import (
	"bufio"
	"bytes"
	"regexp"
	"strings"

	"gorancid/pkg/connect"
	"gorancid/pkg/parse"
)

var (
	ansiRE  = regexp.MustCompile(`\x1b\[[0-9;]*[A-Za-z]`)
	pagerRE = regexp.MustCompile(`(?i)(<[- ]*more[- ]*>|--more--|press any key to continue|press <space> to continue)`)
)

// Parser preserves command output for device types without a dedicated parser yet.
// It strips common terminal noise so the result remains stable enough for VCS diffs.
type Parser struct {
	DeviceType string
}

// New returns a generic parser for device types that do not yet have a dedicated implementation.
func New(deviceType string) *Parser {
	return &Parser{DeviceType: strings.ToLower(deviceType)}
}

// DeviceOpts returns permissive SSH shell parameters for unknown device families
// so native transport can be used instead of legacy login scripts.
func (p *Parser) DeviceOpts() connect.DeviceOpts {
	return connect.DeviceOpts{
		DeviceType:    p.DeviceType,
		PromptPattern: `(?:^|[\r\n])[^\r\n]{1,200}[#>%$]\s*$`,
	}
}

// Parse filters generic command output into stable text without device-specific normalization.
func (p *Parser) Parse(output []byte, _ parse.FilterOpts) (parse.ParsedConfig, error) {
	scanner := bufio.NewScanner(bytes.NewReader(output))
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	lines := make([]string, 0, 256)
	for scanner.Scan() {
		line := scanner.Text()
		line = strings.ReplaceAll(line, "\r", "")
		line = ansiRE.ReplaceAllString(line, "")
		line = pagerRE.ReplaceAllString(line, "")
		line = strings.TrimRight(line, " \t")
		if strings.TrimSpace(line) == "" {
			continue
		}
		lines = append(lines, line)
	}
	if err := scanner.Err(); err != nil {
		return parse.ParsedConfig{}, err
	}
	return parse.ParsedConfig{
		Lines: lines,
		Metadata: map[string]string{
			"parser":      "generic",
			"device_type": p.DeviceType,
		},
	}, nil
}
