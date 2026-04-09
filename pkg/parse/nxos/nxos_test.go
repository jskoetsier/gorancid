package nxos_test

import (
	"strings"
	"testing"

	"gorancid/pkg/config"
	"gorancid/pkg/connect"
	"gorancid/pkg/parse"
	"gorancid/pkg/parse/nxos"
)

func TestRegistered(t *testing.T) {
	p, ok := parse.Lookup("nxos")
	if !ok {
		t.Fatal("nxos parser not registered")
	}
	if _, ok := p.(*nxos.NXOSParser); !ok {
		t.Error("expected *NXOSParser")
	}
}

func TestDeviceOpts(t *testing.T) {
	p := &nxos.NXOSParser{}
	opts := p.DeviceOpts()
	if opts.DeviceType != "nxos" {
		t.Errorf("DeviceType = %q, want nxos", opts.DeviceType)
	}
	if opts.PromptPattern == "" {
		t.Error("PromptPattern should not be empty")
	}
}

func TestShowVersionMetadata(t *testing.T) {
	data := []byte(`show version
Cisco Nexus Operating System (NX-OS) Software, Version 9.3(5)
Cisco Nexus 9000 Series Switch
System image file is: bootflash:///nxos.9.3.5.bin
cisco Nexus9000 C9300v Chassis ("N9K-C9300v")
Intel(R) Xeon(R) CPU E5-2665 0 with 16401060 kB of memory.
`)
	p := &nxos.NXOSParser{}
	cfg, err := p.Parse(data, parse.FilterOpts{})
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}
	if cfg.Metadata["boot_image"] == "" {
		t.Error("expected boot_image metadata to be extracted")
	}
}

func TestPasswordFilterLevel0(t *testing.T) {
	input := []byte("show running-config\nenable password mysecret\nend\n")
	p := &nxos.NXOSParser{}
	cfg, err := p.Parse(input, parse.FilterOpts{FilterPwds: 0})
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}
	for _, line := range cfg.Lines {
		if strings.Contains(line, "enable password") && strings.Contains(line, "mysecret") {
			// Level 0 should NOT filter passwords
			return
		}
	}
	t.Error("expected password to be present at level 0")
}

func TestPasswordFilterLevel1(t *testing.T) {
	input := []byte("show running-config\nenable password mysecret\nusername admin password adminpass\nend\n")
	p := &nxos.NXOSParser{}
	cfg, err := p.Parse(input, parse.FilterOpts{FilterPwds: 1})
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}
	for _, line := range cfg.Lines {
		if strings.Contains(line, "mysecret") {
			t.Errorf("password value should be filtered at level 1, got: %s", line)
		}
		if strings.Contains(line, "adminpass") {
			t.Errorf("username password value should be filtered at level 1, got: %s", line)
		}
	}
}

func TestPasswordFilterLevel2(t *testing.T) {
	input := []byte("show running-config\nenable secret 5 $1$abcde$efghijkLMNOPQRsTUVwxy\nusername admin secret 5 $1$abc\nend\n")
	p := &nxos.NXOSParser{}
	cfg, err := p.Parse(input, parse.FilterOpts{FilterPwds: 2})
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}
	for _, line := range cfg.Lines {
		if strings.Contains(line, "$1$abc") {
			t.Errorf("secret value should be filtered at level 2, got: %s", line)
		}
	}
}

func TestSNMPCommunityFilter(t *testing.T) {
	input := []byte("show running-config\nsnmp-server community public RO\nsnmp-server host 10.0.0.1 traps public\nend\n")
	p := &nxos.NXOSParser{}
	cfg, err := p.Parse(input, parse.FilterOpts{NoCommStr: true})
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}
	for _, line := range cfg.Lines {
		if strings.Contains(line, "snmp-server community") && strings.Contains(line, "public") && !strings.Contains(line, "<removed>") {
			t.Errorf("SNMP community should be filtered, got: %s", line)
		}
	}
}

func TestHeaderRemoval(t *testing.T) {
	input := []byte("show running-config\n!Command: show running-config\n!Time: 2024-01-01 00:00:00\nBuilding configuration...\nCurrent configuration : 1234 bytes\n!\nend\n")
	p := &nxos.NXOSParser{}
	cfg, err := p.Parse(input, parse.FilterOpts{})
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}
	for _, line := range cfg.Lines {
		if strings.Contains(line, "!Command:") {
			t.Error("command header should be removed")
		}
		if strings.Contains(line, "!Time:") {
			t.Error("timestamp should be removed")
		}
		if strings.Contains(line, "Building configuration") {
			t.Error("Building configuration header should be removed")
		}
		if strings.Contains(line, "Current configuration") {
			t.Error("Current configuration header should be removed")
		}
	}
}

func TestEmptyInput(t *testing.T) {
	p := &nxos.NXOSParser{}
	cfg, err := p.Parse([]byte(""), parse.FilterOpts{})
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}
	if len(cfg.Lines) != 0 {
		t.Errorf("expected empty output, got %d lines", len(cfg.Lines))
	}
}

func TestNewSessionNXOS(t *testing.T) {
	_ = connect.NewSession("sw-01", 22, config.Credentials{}, connect.DeviceOpts{DeviceType: "nxos"}, "clogin", true)
}