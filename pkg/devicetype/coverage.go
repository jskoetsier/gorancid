package devicetype

import (
	"strings"

	"gorancid/pkg/parse"
	"gorancid/pkg/parse/generic"
)

var moduleParsers = map[string]string{
	"aeos":      "aeos",
	"fortigate": "fortigate",
	"ios":       "ios",
	"iosxr":     "iosxr",
	"junos":     "junos",
	"nxos":      "nxos",
}

// RegisterMissingParsers ensures every device type in specs has a Go parser
// registered in pkg/parse, mirroring cmd/rancid coverage rules.
func RegisterMissingParsers(specs map[string]DeviceSpec) {
	for name, spec := range specs {
		if _, ok := parse.Lookup(name); ok {
			continue
		}
		if strings.HasPrefix(strings.ToLower(name), "forti") {
			parse.RegisterAlias(name, "fortigate")
			continue
		}
		if spec.Alias != "" {
			if _, ok := parse.Lookup(spec.Alias); ok {
				parse.RegisterAlias(name, spec.Alias)
				continue
			}
		}
		for _, module := range spec.Modules {
			if target, ok := moduleParsers[strings.ToLower(module)]; ok {
				parse.RegisterAlias(name, target)
				goto next
			}
		}
		parse.Register(name, generic.New(name))
	next:
	}
}
