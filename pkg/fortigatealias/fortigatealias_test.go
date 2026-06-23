package fortigatealias_test

import (
	"testing"

	"gorancid/pkg/fortigatealias"
)

func TestUsesFortiGate(t *testing.T) {
	yes := []string{"fortiscp", "FORTISCP", " fortiscp ", "fortigate-full"}
	for _, typ := range yes {
		if !fortigatealias.UsesFortiGate(typ) {
			t.Errorf("UsesFortiGate(%q) = false, want true", typ)
		}
	}

	no := []string{
		"fortigate", // registered directly, not an alias
		"fortiswitch",
		"forti-ap",
		"forti-web",
		"forti-mgmt",
		"fortimail",
		"fortinet",
		"doesnotexist",
	}
	for _, typ := range no {
		if fortigatealias.UsesFortiGate(typ) {
			t.Errorf("UsesFortiGate(%q) = true, want false", typ)
		}
	}
}

func TestBaseType(t *testing.T) {
	base, ok := fortigatealias.BaseType("fortiscp")
	if !ok || base != "fortigate" {
		t.Fatalf("BaseType(fortiscp) = %q, %v; want fortigate, true", base, ok)
	}
	if _, ok := fortigatealias.BaseType("fortiswitch"); ok {
		t.Fatal("fortiswitch should not map to fortigate")
	}
}
