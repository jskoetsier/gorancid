package main

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"gorancid/pkg/config"
	"gorancid/pkg/devicetype"
)

func TestSplitCommands(t *testing.T) {
	got := splitCommands("show version; show running-config ; ; write term")
	want := []string{"show version", "show running-config", "write term"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("splitCommands() = %#v, want %#v", got, want)
	}
}

func TestFirstNativeTransportOrder(t *testing.T) {
	kind, port, ok := firstNativeTransport([]string{"telnet", "ssh:2222"})
	if !ok {
		t.Fatal("expected transport")
	}
	if kind != "telnet" || port != 23 {
		t.Fatalf("got %s:%d, want telnet:23 (first method wins)", kind, port)
	}
	kind, port, ok = firstNativeTransport([]string{"ssh:2222", "telnet"})
	if !ok || kind != "ssh" || port != 2222 {
		t.Fatalf("got %s:%d, want ssh:2222", kind, port)
	}
}

func TestCanUseNative(t *testing.T) {
	if !canUseNative("cisco", []string{"ssh"}) {
		t.Fatal("expected cisco alias to use native parser")
	}
	if canUseNative("unknown", []string{"ssh"}) {
		t.Fatal("unexpected native support for unknown type")
	}
	if !canUseNative("ios", []string{"telnet"}) {
		t.Fatal("expected native telnet support for ios when parser exposes DeviceOpts")
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

func TestResolveSysconfDir(t *testing.T) {
	t.Setenv("RANCID_SYSCONFDIR", "/env/etc")
	if got := resolveSysconfDir("/flag/rancid.conf"); got != "/env/etc" {
		t.Fatalf("env sysconfdir = %q", got)
	}

	t.Setenv("RANCID_SYSCONFDIR", "")
	if got := resolveSysconfDir("/etc/rancid/rancid.conf"); got != "/etc/rancid" {
		t.Fatalf("derived sysconfdir = %q", got)
	}
	if got := resolveSysconfDir(""); got != "/usr/local/rancid/etc" {
		t.Fatalf("default sysconfdir = %q", got)
	}
}

func TestEnsureParserCoverage(t *testing.T) {
	specs := map[string]devicetype.DeviceSpec{
		"fortigate-full": {Type: "fortigate-full", Modules: []string{"fortigate"}},
		"juniper-srx":    {Type: "juniper-srx", Alias: "junos"},
		"fortiscp":       {Type: "fortiscp"},
	}

	devicetype.RegisterMissingParsers(specs)

	if !canUseNative("fortigate-full", []string{"ssh"}) {
		t.Fatal("expected fortigate-full to use native parser coverage")
	}
	if !canUseNative("juniper-srx", []string{"ssh"}) {
		t.Fatal("expected juniper-srx alias to use native parser coverage")
	}
	if !canUseNative("fortiscp", []string{"ssh"}) {
		t.Fatal("expected fortiscp to inherit native FortiGate parser coverage")
	}
}

