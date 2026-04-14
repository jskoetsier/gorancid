package main

import (
	"strings"

	"gorancid/pkg/devicetype"
	"gorancid/pkg/parse"
	"gorancid/pkg/parse/generic"
)

var moduleParsers = map[string]string{
	"fortigate": "fortigate",
	"ios":       "ios",
	"iosxr":     "iosxr",
	"junos":     "junos",
	"nxos":      "nxos",
}

func ensureParserCoverage(specs map[string]devicetype.DeviceSpec) {
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
