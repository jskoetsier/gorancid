package main

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"gorancid/pkg/config"
)

func TestSplitCommands(t *testing.T) {
	got := splitCommands("show version; show running-config ; ; write term")
	want := []string{"show version", "show running-config", "write term"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("splitCommands() = %#v, want %#v", got, want)
	}
}

func TestFirstSSHMethod(t *testing.T) {
	port, ok := firstSSHMethod([]string{"telnet", "ssh:2222"})
	if !ok {
		t.Fatal("expected ssh method to be found")
	}
	if port != 2222 {
		t.Fatalf("port = %d, want 2222", port)
	}
}

func TestCanUseNative(t *testing.T) {
	if !canUseNative("cisco", []string{"ssh"}) {
		t.Fatal("expected cisco alias to use native parser")
	}
	if canUseNative("unknown", []string{"ssh"}) {
		t.Fatal("unexpected native support for unknown type")
	}
	if canUseNative("ios", []string{"telnet"}) {
		t.Fatal("unexpected native support for telnet-only method")
	}
}

func TestFindDeviceRouterDBOverride(t *testing.T) {
	cfg := config.Config{}
	path := filepath.Join("..", "..", "pkg", "config", "testdata", "router.db")
	dev, _, err := findDevice("edge-fw-01.example.com", cfg, path)
	if err != nil {
		t.Fatalf("findDevice: %v", err)
	}
	if dev.Type != "fortigate" {
		t.Fatalf("device type = %q, want fortigate", dev.Type)
	}
}

func TestDeviceOptsEnableForCisco(t *testing.T) {
	opts := deviceOpts("cisco", config.Credentials{EnablePwd: "secret"}, 15, false, false)
	if opts.EnableCmd != "enable" {
		t.Fatalf("EnableCmd = %q, want enable", opts.EnableCmd)
	}
}

func TestResolveConfigPath(t *testing.T) {
	t.Setenv("RANCID_CONF", "/env/rancid.conf")

	if got := resolveConfigPath("/flag/rancid.conf", ""); got != "/flag/rancid.conf" {
		t.Fatalf("primary flag path = %q", got)
	}
	if got := resolveConfigPath("", "/alt/rancid.conf"); got != "/alt/rancid.conf" {
		t.Fatalf("secondary flag path = %q", got)
	}
	if got := resolveConfigPath("", ""); got != "/env/rancid.conf" {
		t.Fatalf("env path = %q", got)
	}

	_ = os.Unsetenv("RANCID_CONF")
	if got := resolveConfigPath("", ""); got != "/usr/local/rancid/etc/rancid.conf" {
		t.Fatalf("default path = %q", got)
	}
}
