package iosxr_test

import (
	"strings"
	"testing"

	"gorancid/pkg/config"
	"gorancid/pkg/connect"
	"gorancid/pkg/parse"
	"gorancid/pkg/parse/iosxr"
)

func TestRegistered(t *testing.T) {
	p, ok := parse.Lookup("iosxr")
	if !ok {
		t.Fatal("iosxr parser not registered")
	}
	if _, ok := p.(*iosxr.IOSXRParser); !ok {
		t.Error("expected *IOSXRParser")
	}
}

func TestDeviceOpts(t *testing.T) {
	p := &iosxr.IOSXRParser{}
	opts := p.DeviceOpts()
	if opts.DeviceType != "iosxr" {
		t.Errorf("DeviceType = %q, want iosxr", opts.DeviceType)
	}
}

func TestEmptyInput(t *testing.T) {
	p := &iosxr.IOSXRParser{}
	cfg, err := p.Parse([]byte(""), parse.FilterOpts{})
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}
	if len(cfg.Lines) != 0 {
		t.Errorf("expected empty output, got %d lines", len(cfg.Lines))
	}
}

func TestPasswordFilterLevel0(t *testing.T) {
	input := []byte("show running-config\nenable password mysecret\nend\n")
	p := &iosxr.IOSXRParser{}
	cfg, err := p.Parse(input, parse.FilterOpts{FilterPwds: 0})
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}
	found := false
	for _, line := range cfg.Lines {
		if strings.Contains(line, "enable password") && strings.Contains(line, "mysecret") {
			found = true
		}
	}
	if !found {
		t.Error("password should NOT be filtered at level 0")
	}
}

func TestPasswordFilterLevel1(t *testing.T) {
	input := []byte("show running-config\nenable password mysecret\nend\n")
	p := &iosxr.IOSXRParser{}
	cfg, err := p.Parse(input, parse.FilterOpts{FilterPwds: 1})
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}
	for _, line := range cfg.Lines {
		if strings.Contains(line, "mysecret") {
			t.Errorf("password value should be filtered at level 1, got: %s", line)
		}
	}
}

func TestSNMPCommunityFilter(t *testing.T) {
	input := []byte("show running-config\nsnmp-server community public RO\nend\n")
	p := &iosxr.IOSXRParser{}
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
	input := []byte("show running-config\nBuilding configuration...\n! Last configuration change at 14:32:18 UTC\nend\n")
	p := &iosxr.IOSXRParser{}
	cfg, err := p.Parse(input, parse.FilterOpts{})
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}
	for _, line := range cfg.Lines {
		if strings.Contains(line, "Building configuration") {
			t.Error("Building configuration header should be removed")
		}
		if strings.Contains(line, "Last configuration change") {
			t.Error("timestamp header should be removed")
		}
	}
}

func TestNtpClockPeriodRemoval(t *testing.T) {
	input := []byte("show running-config\nntp clock-period 17179869\nend\n")
	p := &iosxr.IOSXRParser{}
	cfg, err := p.Parse(input, parse.FilterOpts{})
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}
	for _, line := range cfg.Lines {
		if strings.Contains(line, "ntp clock-period") {
			t.Error("ntp clock-period should be removed")
		}
	}
}

func TestNewSessionIOSXR(t *testing.T) {
	_ = connect.NewSession("router-01", 22, config.Credentials{}, connect.DeviceOpts{DeviceType: "iosxr"}, "clogin", true)
}