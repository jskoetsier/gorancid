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
	if ios.LoginScript != "clogin" {
		t.Errorf("ios.LoginScript = %q, want clogin", ios.LoginScript)
	}
	if len(ios.Commands) != 2 {
		t.Errorf("ios.Commands count = %d, want 2", len(ios.Commands))
	}
	if ios.Commands[0].CLI != "show version" {
		t.Errorf("ios.Commands[0].CLI = %q", ios.Commands[0].CLI)
	}

	// custom type only in conf
	myios, ok := specs["myios"]
	if !ok {
		t.Fatal("myios type not found")
	}
	if myios.LoginScript != "clogin" {
		t.Errorf("myios.LoginScript = %q, want clogin", myios.LoginScript)
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

	spec, ok := devicetype.Lookup(specs, "cat5k")
	if !ok {
		t.Fatal("cat5k alias not resolved")
	}
	if spec.LoginScript != "clogin" {
		t.Errorf("resolved alias LoginScript = %q, want clogin", spec.LoginScript)
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