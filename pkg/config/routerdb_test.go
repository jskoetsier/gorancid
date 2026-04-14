package config_test

import (
	"testing"

	"gorancid/pkg/config"
)

func TestLoadRouterDB(t *testing.T) {
	devices, err := config.LoadRouterDB("testdata/router.db")
	if err != nil {
		t.Fatalf("LoadRouterDB: %v", err)
	}
	if len(devices) != 4 {
		t.Fatalf("got %d devices, want 4", len(devices))
	}
	if devices[0].Hostname != "core-sw-01.example.com" {
		t.Errorf("devices[0].Hostname = %q", devices[0].Hostname)
	}
	if devices[0].Type != "ios" {
		t.Errorf("devices[0].Type = %q", devices[0].Type)
	}
	if devices[0].Status != "up" {
		t.Errorf("devices[0].Status = %q", devices[0].Status)
	}
	if devices[3].Status != "down" {
		t.Errorf("devices[3].Status = %q, want down", devices[3].Status)
	}
}

func TestLoadRouterDBMalformed(t *testing.T) {
	_, err := config.LoadRouterDB("testdata/router_bad.db")
	if err == nil {
		t.Error("expected error for malformed router.db, got nil")
	}
}

func TestLoadRouterDBSemicolon(t *testing.T) {
	devices, err := config.LoadRouterDB("testdata/router_semicolon.db")
	if err != nil {
		t.Fatalf("LoadRouterDB: %v", err)
	}
	if len(devices) != 2 {
		t.Fatalf("got %d devices, want 2", len(devices))
	}
	if devices[1].Hostname != "ix5-rtr-p01" {
		t.Errorf("devices[1].Hostname = %q", devices[1].Hostname)
	}
	if devices[1].Type != "cisco" {
		t.Errorf("devices[1].Type = %q", devices[1].Type)
	}
	if devices[1].Status != "up" {
		t.Errorf("devices[1].Status = %q", devices[1].Status)
	}
}
