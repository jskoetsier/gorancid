// pkg/config/cloginrc_test.go
package config_test

import (
	"testing"

	"gorancid/pkg/config"
)

func TestLoadCloginrc(t *testing.T) {
	cs, err := config.LoadCloginrc("testdata/cloginrc")
	if err != nil {
		t.Fatalf("LoadCloginrc: %v", err)
	}

	// specific host gets its specific password
	creds := cs.Lookup("core-sw-01.example.com")
	if creds.Password != "s3cr3t" {
		t.Errorf("password = %q, want s3cr3t", creds.Password)
	}
	if creds.Username != "netops" {
		t.Errorf("username = %q, want netops", creds.Username)
	}
	if creds.EnablePwd != "en4ble" {
		t.Errorf("enablepassword = %q, want en4ble", creds.EnablePwd)
	}
	if len(creds.Methods) != 2 || creds.Methods[0] != "ssh" {
		t.Errorf("methods = %v, want [ssh telnet]", creds.Methods)
	}

	// wildcard host gets the wildcard password
	creds2 := cs.Lookup("edge-fw-01.example.com")
	if creds2.Password != "d3fault" {
		t.Errorf("wildcard password = %q, want d3fault", creds2.Password)
	}

	// specific method override
	creds3 := cs.Lookup("old-router.example.com")
	if len(creds3.Methods) != 1 || creds3.Methods[0] != "telnet" {
		t.Errorf("old-router methods = %v, want [telnet]", creds3.Methods)
	}
}

func TestLoadCloginrcMultiValuePassword(t *testing.T) {
	cs, err := config.LoadCloginrc("testdata/cloginrc_multivalue")
	if err != nil {
		t.Fatalf("LoadCloginrc: %v", err)
	}

	creds := cs.Lookup("ix5-rtr-p01-re0")
	if creds.Username != "rancid" {
		t.Fatalf("username = %q, want rancid", creds.Username)
	}
	if creds.Password != "loginpass" {
		t.Fatalf("password = %q, want loginpass", creds.Password)
	}
	if creds.EnablePwd != "enablepass" {
		t.Fatalf("enable password = %q, want enablepass", creds.EnablePwd)
	}
	if len(creds.Methods) != 1 || creds.Methods[0] != "ssh" {
		t.Fatalf("methods = %v, want [ssh]", creds.Methods)
	}
}

func TestLoadCloginrcUserPasswordOverridesPassword(t *testing.T) {
	cs, err := config.LoadCloginrc("testdata/cloginrc_userpassword")
	if err != nil {
		t.Fatalf("LoadCloginrc: %v", err)
	}

	creds := cs.Lookup("ix5-rtr-p01-re1")
	if creds.Password != "userpass" {
		t.Fatalf("password = %q, want userpass", creds.Password)
	}
}
