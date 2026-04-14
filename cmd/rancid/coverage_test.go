package main

import (
	"testing"

	"gorancid/pkg/devicetype"
	"gorancid/pkg/parse"

	_ "gorancid/pkg/parse/fortigate"
	_ "gorancid/pkg/parse/ios"
	_ "gorancid/pkg/parse/iosxr"
	_ "gorancid/pkg/parse/junos"
	_ "gorancid/pkg/parse/nxos"
)

func TestEnsureParserCoverage(t *testing.T) {
	specs := map[string]devicetype.DeviceSpec{
		"cisco-nx":   {Type: "cisco-nx", Modules: []string{"nxos"}},
		"juniper":    {Type: "juniper", Alias: "junos"},
		"riverstone": {Type: "riverstone"},
	}

	ensureParserCoverage(specs)

	for _, name := range []string{"cisco-nx", "juniper", "riverstone"} {
		if _, ok := parse.Lookup(name); !ok {
			t.Fatalf("parser for %s not registered", name)
		}
	}
}
