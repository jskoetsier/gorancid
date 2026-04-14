package connect

import (
	"context"
	"testing"

	"gorancid/pkg/config"
)

func TestSSHSessionNotConnected(t *testing.T) {
	s := &SSHSession{}
	_, err := s.RunCommand(context.Background(), "show version")
	if err == nil {
		t.Error("expected error when not connected")
	}
}

func TestSSHSessionClose(t *testing.T) {
	s := &SSHSession{}
	err := s.Close()
	if err != nil {
		t.Errorf("Close on unconnected session should not error: %v", err)
	}
}

func TestExpectSessionConnect(t *testing.T) {
	e := &ExpectSession{}
	err := e.Connect(context.Background())
	if err != nil {
		t.Errorf("ExpectSession.Connect should be no-op: %v", err)
	}
}

func TestExpectSessionClose(t *testing.T) {
	e := &ExpectSession{}
	err := e.Close()
	if err != nil {
		t.Errorf("ExpectSession.Close should be no-op: %v", err)
	}
}

func TestNewSessionExpect(t *testing.T) {
	s := NewSession("sw-01", 22, config.Credentials{}, DeviceOpts{}, "clogin", false)
	if _, ok := s.(*ExpectSession); !ok {
		t.Error("expected ExpectSession when goParserAvailable=false")
	}
}

func TestNewSessionSSH(t *testing.T) {
	s := NewSession("sw-01", 22, config.Credentials{}, DeviceOpts{}, "clogin", true)
	if _, ok := s.(*SSHSession); !ok {
		t.Error("expected SSHSession when goParserAvailable=true")
	}
}

func TestNewSessionExpectWhenOnlyTelnetMethod(t *testing.T) {
	s := NewSession("sw-01", 22, config.Credentials{Methods: []string{"telnet"}}, DeviceOpts{}, "clogin", true)
	if _, ok := s.(*ExpectSession); !ok {
		t.Error("expected ExpectSession when SSH is unavailable")
	}
}

func TestSSHAuthMethods(t *testing.T) {
	if methods := sshAuthMethods(config.Credentials{}); len(methods) != 0 {
		t.Fatalf("expected no auth methods without password, got %d", len(methods))
	}
	if methods := sshAuthMethods(config.Credentials{Password: "secret"}); len(methods) != 2 {
		t.Fatalf("expected password and keyboard-interactive methods, got %d", len(methods))
	}
}
