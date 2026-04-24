// Package asa implements the Cisco ASA device parser for gorancid.
// It reuses the IOS parser for output filtering but provides ASA-specific
// connection parameters (terminal pager 0 instead of terminal length 0).
package asa

import (
	"gorancid/pkg/connect"
	"gorancid/pkg/parse"
	"gorancid/pkg/parse/ios"
)

func init() {
	parse.Register("asa", &ASAParser{})
	parse.RegisterAlias("cisco-asa", "asa")
}

// ASAParser implements parse.Parser for Cisco ASA devices.
// It delegates output processing to the IOS parser since command
// structure and filtering are identical.
type ASAParser struct{}

// DeviceOpts returns connection parameters for the SSH connector.
// ASA uses "terminal pager 0" instead of "terminal length 0".
func (p *ASAParser) DeviceOpts() connect.DeviceOpts {
	return connect.DeviceOpts{
		DeviceType:       "asa",
		PromptPattern:    `(?:^|[\r\n])[\w./:-]+(?:\([^)]+\))*[#>]\s*$`,
		SetupCommands:    []string{"terminal pager 0"},
		EnableCmd:        "enable",
		DisablePagingCmd: "terminal pager 0",
	}
}

// Parse delegates to the IOS parser since output format and filtering
// requirements are identical.
func (p *ASAParser) Parse(output []byte, filter parse.FilterOpts) (parse.ParsedConfig, error) {
	return (&ios.IOSParser{}).Parse(output, filter)
}
