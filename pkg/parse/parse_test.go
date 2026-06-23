package parse_test

import (
	"testing"

	_ "gorancid/pkg/parse/fortigate"
	"gorancid/pkg/parse"
)

func TestLookupFortiPrefixFallback(t *testing.T) {
	if _, ok := parse.Lookup("fortigate"); !ok {
		t.Fatal("expected fortigate parser to be registered")
	}
	parser, ok := parse.Lookup("fortiscp")
	if !ok {
		t.Fatal("expected fortiscp to fall back to fortigate parser")
	}
	fg, ok := parse.Lookup("fortigate")
	if !ok {
		t.Fatal("expected fortigate parser")
	}
	if parser != fg {
		t.Fatal("fortiscp fallback should return the same parser as fortigate")
	}
	if _, ok := parse.Lookup("fortiswitch"); ok {
		t.Fatal("fortiswitch must not fall back to fortigate parser")
	}
}

func TestLookupUnknownType(t *testing.T) {
	if _, ok := parse.Lookup("doesnotexist"); ok {
		t.Fatal("expected unknown type to miss")
	}
}
