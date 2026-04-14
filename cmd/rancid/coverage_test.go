package main

import (
	"testing"

	_ "gorancid/pkg/collect" // register parsers via collect/parsers.go
	"gorancid/pkg/devicetype"
	"gorancid/pkg/parse"
)

func TestRegisterMissingParsers(t *testing.T) {
	specs := map[string]devicetype.DeviceSpec{
		"cisco-nx":   {Type: "cisco-nx", Modules: []string{"nxos"}},
		"juniper":    {Type: "juniper", Alias: "junos"},
		"riverstone": {Type: "riverstone"},
	}

	devicetype.RegisterMissingParsers(specs)

	for _, name := range []string{"cisco-nx", "juniper", "riverstone"} {
		if _, ok := parse.Lookup(name); !ok {
			t.Fatalf("parser for %s not registered", name)
		}
	}
}
