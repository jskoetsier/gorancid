package devicetype_test

import (
	"testing"

	"gorancid/pkg/devicetype"
)

func TestLoad(t *testing.T) {
	specs, err := devicetype.Load(
		"testdata/rancid.types.base",
		"testdata/rancid.types.conf",
	)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	// ios is in base
	ios, ok := specs["ios"]
	if !ok {
		t.Fatal("ios type not found")
	}
	if len(ios.Commands) != 2 {
		t.Errorf("ios.Commands count = %d, want 2", len(ios.Commands))
	}
	if ios.Commands[0].CLI != "show version" {
		t.Errorf("ios.Commands[0].CLI = %q", ios.Commands[0].CLI)
	}

	// custom type only in conf
	_, ok = specs["myios"]
	if !ok {
		t.Fatal("myios type not found")
	}
}

func TestLookupAlias(t *testing.T) {
	specs, err := devicetype.Load(
		"testdata/rancid.types.base",
		"testdata/rancid.types.conf",
	)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	_, ok := devicetype.Lookup(specs, "cat5k")
	if !ok {
		t.Fatal("cat5k alias not resolved")
	}
}

func TestLookupMissing(t *testing.T) {
	specs, _ := devicetype.Load(
		"testdata/rancid.types.base",
		"testdata/rancid.types.conf",
	)
	_, ok := devicetype.Lookup(specs, "doesnotexist")
	if ok {
		t.Error("expected Lookup to return false for unknown type")
	}
}