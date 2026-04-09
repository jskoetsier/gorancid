package config_test

import (
	"testing"

	"gorancid/pkg/config"
)

func TestLoad(t *testing.T) {
	cfg, err := config.Load("testdata/rancid.conf")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.BaseDir != "/usr/local/rancid/var" {
		t.Errorf("BaseDir = %q, want /usr/local/rancid/var", cfg.BaseDir)
	}
	if cfg.LogDir != "/usr/local/rancid/var/logs" {
		t.Errorf("LogDir = %q, want /usr/local/rancid/var/logs", cfg.LogDir)
	}
	if cfg.RepoRoot != "/usr/local/rancid/var/CVS" {
		t.Errorf("RepoRoot = %q", cfg.RepoRoot)
	}
	if len(cfg.Groups) != 3 || cfg.Groups[0] != "core" {
		t.Errorf("Groups = %v, want [core edge dmz]", cfg.Groups)
	}
	if cfg.ParCount != 10 {
		t.Errorf("ParCount = %d, want 10", cfg.ParCount)
	}
	if cfg.FilterPwds != config.FilterYes {
		t.Errorf("FilterPwds = %v, want FilterYes", cfg.FilterPwds)
	}
	if !cfg.NoCommStr {
		t.Error("NoCommStr should be true")
	}
}

func TestLoadDefaults(t *testing.T) {
	cfg, err := config.Load("testdata/rancid_minimal.conf")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.ParCount != 5 {
		t.Errorf("ParCount default = %d, want 5", cfg.ParCount)
	}
	if cfg.OldTime != 24 {
		t.Errorf("OldTime default = %d, want 24", cfg.OldTime)
	}
	if cfg.MaxRounds != 4 {
		t.Errorf("MaxRounds default = %d, want 4", cfg.MaxRounds)
	}
}
